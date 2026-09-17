package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FindPendingBackgroundTask returns one pending task with the supplied semantic
// identity inside an already-serialized enqueue transaction. Running work is
// deliberately excluded: a producer that commits after a running task's final
// scan still needs a fresh pending wake.
func (s *Store) FindPendingBackgroundTask(q Querier, operationID, kind, subjectKind, subjectID, inputKey, resourceClass string) (BackgroundTask, bool, error) {
	if q == nil {
		return BackgroundTask{}, false, errors.New("background task querier is required")
	}
	if operationID == "" {
		return BackgroundTask{}, false, errors.New("background task operation id is required")
	}
	if kind == "" {
		return BackgroundTask{}, false, errors.New("background task kind is required")
	}
	if inputKey == "" {
		return BackgroundTask{}, false, errors.New("background task input key is required")
	}
	if resourceClass == "" {
		resourceClass = "default"
	}

	task, err := scanBackgroundTask(q.QueryRow(`
		SELECT id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
		       resource_class, priority, status, available_at, lease_owner, lease_expires_at,
		       created_at, started_at, finished_at, attempt_count, max_attempts,
		       last_error_code, last_error_message
		FROM background_tasks
		WHERE operation_id = ? AND status = 'pending'
		  AND kind = ? AND subject_kind = ? AND subject_id = ? AND input_key = ?
		  AND resource_class = ?
		ORDER BY created_at, id
		LIMIT 1
	`, operationID, kind, subjectKind, subjectID, inputKey, resourceClass))
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundTask{}, false, nil
	}
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("find pending background task: %w", err)
	}
	return task, true, nil
}

// PostponePendingBackgroundTask moves a pending task's claim time later without
// changing running work. Callers use this inside the same serialized producer
// transaction that found the task, so a trailing-edge debounce stays atomic
// with the domain mutation that justified postponing the wake.
func (s *Store) PostponePendingBackgroundTask(q Querier, taskID string, availableAt time.Time) (bool, error) {
	if q == nil {
		return false, errors.New("background task querier is required")
	}
	if taskID == "" {
		return false, errors.New("background task id is required")
	}
	if availableAt.IsZero() {
		return false, errors.New("background task availability is required")
	}
	availableAt = availableAt.UTC()
	availableAtValue := workTimeValue(availableAt)
	result, err := q.Exec(`
		UPDATE background_tasks
		SET available_at = ?
		WHERE id = ? AND status = 'pending' AND available_at < ?
	`, availableAtValue, taskID, availableAtValue)
	if err != nil {
		return false, fmt.Errorf("postpone pending background task: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("postpone pending background task rows affected: %w", err)
	}
	return rows == 1, nil
}
