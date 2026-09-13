package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// AttachBackgroundTaskToOperation atomically attaches one stable child task and
// its initial recovery checkpoint to an existing logical operation. Repeating
// the same task ID with the same immutable work description is idempotent even
// after that child or its operation becomes terminal; the retry never rewrites
// recovery state that may already have advanced. A new child is accepted only
// while the operation is pending or running and while its declared child count
// has not been reached.
func (s *Store) AttachBackgroundTaskToOperation(operationID string, checkpointJSON []byte, task NewBackgroundTask) (BackgroundTask, bool, error) {
	if s == nil || s.DB == nil {
		return BackgroundTask{}, false, errors.New("background operation store is required")
	}
	if operationID == "" {
		return BackgroundTask{}, false, errors.New("background operation id is required")
	}
	if task.ID == "" {
		return BackgroundTask{}, false, errors.New("background task id is required")
	}
	if task.DedupeKey == "" {
		return BackgroundTask{}, false, errors.New("background task dedupe key is required")
	}
	if task.Kind == "" {
		return BackgroundTask{}, false, errors.New("background task kind is required")
	}
	if task.ResourceClass == "" {
		task.ResourceClass = "default"
	}
	if task.MaxAttempts == 0 {
		task.MaxAttempts = 3
	}
	if task.MaxAttempts < 1 {
		return BackgroundTask{}, false, errors.New("background task max attempts must be positive")
	}
	cleanupJSON, err := encodeBackgroundTaskCleanup(task.TerminalFailureCleanup)
	if err != nil {
		return BackgroundTask{}, false, err
	}
	task.OperationID = operationID

	tx, err := s.Begin()
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("begin background child attachment: %w", err)
	}
	defer tx.Rollback()

	// Acquire SQLite writer ownership before reading task/operation state. This
	// avoids a stale WAL read snapshot that cannot be upgraded if another worker
	// commits between validation and insertion.
	res, err := tx.Exec(`
		UPDATE background_operations
		SET progress_total = progress_total
		WHERE id = ?
	`, operationID)
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("lock background operation for child attachment: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("lock background operation rows affected: %w", err)
	}
	if rows != 1 {
		return BackgroundTask{}, false, errors.New("background operation is missing")
	}

	existing, err := scanBackgroundTask(tx.QueryRow(`
		SELECT id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
		       resource_class, priority, status, available_at, lease_owner, lease_expires_at,
		       created_at, started_at, finished_at, attempt_count, max_attempts,
		       last_error_code, last_error_message
		FROM background_tasks
		WHERE id = ?
	`, task.ID))
	if err == nil {
		var existingCleanup string
		if err := tx.QueryRow(`SELECT terminal_cleanup_json FROM background_tasks WHERE id = ?`, task.ID).Scan(&existingCleanup); err != nil {
			return BackgroundTask{}, false, fmt.Errorf("read existing background child cleanup: %w", err)
		}
		if !backgroundTaskAttachmentMatches(existing, existingCleanup, task, cleanupJSON) {
			return BackgroundTask{}, false, errors.New("background task id is already attached with different immutable input")
		}
		return existing, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return BackgroundTask{}, false, fmt.Errorf("inspect existing background child attachment: %w", err)
	}

	var status string
	var progressTotal, attached int64
	if err := tx.QueryRow(`
		SELECT status, progress_total,
		       (SELECT count(*) FROM background_tasks WHERE operation_id = background_operations.id)
		FROM background_operations
		WHERE id = ?
	`, operationID).Scan(&status, &progressTotal, &attached); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("inspect background operation child capacity: %w", err)
	}
	if status != string(BackgroundWorkPending) && status != string(BackgroundWorkRunning) {
		return BackgroundTask{}, false, errors.New("background operation no longer accepts new child tasks")
	}
	if progressTotal > 0 && attached >= progressTotal {
		return BackgroundTask{}, false, errors.New("background operation already has its declared child count")
	}

	attachedTask, created, err := s.EnqueueBackgroundTask(tx, task)
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("attach background child task: %w", err)
	}
	if !created {
		return BackgroundTask{}, false, errors.New("attach background child task: scoped dedupe key already active")
	}
	if err := s.setBackgroundTaskCheckpoint(tx, attachedTask.ID, checkpointJSON); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("persist background child checkpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("commit background child attachment: %w", err)
	}
	return attachedTask, true, nil
}

func backgroundTaskAttachmentMatches(existing BackgroundTask, existingCleanup string, task NewBackgroundTask, cleanupJSON string) bool {
	return existing.OperationID == task.OperationID &&
		existing.DedupeKey == task.DedupeKey &&
		existing.Kind == task.Kind &&
		existing.SubjectKind == task.SubjectKind &&
		existing.SubjectID == task.SubjectID &&
		existing.InputKey == task.InputKey &&
		existing.ResourceClass == task.ResourceClass &&
		existing.Priority == task.Priority &&
		existing.MaxAttempts == task.MaxAttempts &&
		existingCleanup == cleanupJSON
}
