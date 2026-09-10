package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BackgroundOperationCancellation describes the ownership state at the exact
// transaction where an operation cancellation won. RunningTasks lets a producer
// distinguish work that is safe to clean immediately from work whose worker may
// still be unwinding side effects after losing its lease.
type BackgroundOperationCancellation struct {
	Canceled     bool
	RunningTasks int64
}

// CancelBackgroundTask durably cancels one pending or running task. Canceling a
// running task also closes its live attempt and clears the lease, so the worker
// loses ownership on its next renewal and must stop side effects. Terminal tasks
// are left unchanged and report canceled=false.
func (s *Store) CancelBackgroundTask(taskID string, canceledAt time.Time) (canceled bool, err error) {
	if s == nil || s.DB == nil {
		return false, errors.New("background task store is required")
	}
	if taskID == "" {
		return false, errors.New("background task id is required")
	}
	canceledAt = normalizeWorkTime(canceledAt)

	tx, err := s.Begin()
	if err != nil {
		return false, fmt.Errorf("begin background task cancellation: %w", err)
	}
	defer tx.Rollback()

	var attemptNumber int
	var operationID sql.NullString
	if err := tx.QueryRow(`
		UPDATE background_tasks
		SET status = 'canceled',
		    finished_at = ?,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    last_error_code = '',
		    last_error_message = ''
		WHERE id = ? AND status IN ('pending', 'running')
		RETURNING attempt_count, operation_id
	`, workTimeValue(canceledAt), taskID).Scan(&attemptNumber, &operationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("cancel background task: %w", err)
	}

	if attemptNumber > 0 {
		if _, err := tx.Exec(`
			UPDATE background_task_attempts
			SET finished_at = ?, outcome = 'canceled', error_code = '', error_message = ''
			WHERE task_id = ? AND attempt_number = ? AND outcome = 'running'
		`, workTimeValue(canceledAt), taskID, attemptNumber); err != nil {
			return false, fmt.Errorf("cancel background task attempt: %w", err)
		}
	}
	if err := refreshBackgroundOperation(tx, operationID.String, canceledAt); err != nil {
		return false, fmt.Errorf("refresh background operation after task cancellation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit background task cancellation: %w", err)
	}
	return true, nil
}

// CancelBackgroundOperation preserves the boolean cancellation API for callers
// that do not own external side effects. Producers that need to decide cleanup
// ownership should use CancelBackgroundOperationWithDetails.
func (s *Store) CancelBackgroundOperation(operationID string, canceledAt time.Time) (bool, error) {
	result, err := s.CancelBackgroundOperationWithDetails(operationID, canceledAt)
	return result.Canceled, err
}

// CancelBackgroundOperationWithDetails durably cancels one active logical
// operation and all of its pending/running child tasks in the same transaction.
// Marking the parent canceled first makes cancellation sticky: the schema rejects
// any later child enqueue beneath that canceled operation. Existing completed or
// failed history is kept for aggregate diagnostics while active attempts are
// closed as canceled. Once a producer has atomically persisted a success result,
// cancellation is too late even if final child completion bookkeeping has not run.
func (s *Store) CancelBackgroundOperationWithDetails(operationID string, canceledAt time.Time) (result BackgroundOperationCancellation, err error) {
	if s == nil || s.DB == nil {
		return result, errors.New("background task store is required")
	}
	if operationID == "" {
		return result, errors.New("background operation id is required")
	}
	canceledAt = normalizeWorkTime(canceledAt)
	canceledAtValue := workTimeValue(canceledAt)

	tx, err := s.Begin()
	if err != nil {
		return result, fmt.Errorf("begin background operation cancellation: %w", err)
	}
	defer tx.Rollback()

	// Acquire the operation as an active row and make its cancellation visible to
	// all later statements in this transaction. A concurrent enqueue is serialized
	// by SQLite and, after this commits, is rejected by the trigger. A persisted
	// result is the producer's atomic commit boundary and therefore wins the race.
	res, err := tx.Exec(`
		UPDATE background_operations
		SET status = 'canceled',
		    started_at = COALESCE(started_at, ?),
		    finished_at = ?,
		    error_code = '',
		    error_message = ''
		WHERE id = ?
		  AND status IN ('pending', 'running')
		  AND COALESCE(result_json, '') = ''
	`, canceledAtValue, canceledAtValue, operationID)
	if err != nil {
		return result, fmt.Errorf("cancel background operation: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return result, fmt.Errorf("cancel background operation rows affected: %w", err)
	}
	if rows == 0 {
		return result, nil
	}
	result.Canceled = true

	// Capture current ownership before clearing task leases. StartedAt cannot be
	// used for this decision because a retried task can be pending after an earlier
	// attempt while still carrying its historical start timestamp.
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM background_tasks
		WHERE operation_id = ? AND status = 'running'
	`, operationID).Scan(&result.RunningTasks); err != nil {
		return BackgroundOperationCancellation{}, fmt.Errorf("count running background tasks during cancellation: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE background_task_attempts
		SET finished_at = ?, outcome = 'canceled', error_code = '', error_message = ''
		WHERE outcome = 'running'
		  AND task_id IN (
			SELECT id FROM background_tasks
			WHERE operation_id = ? AND status = 'running'
		  )
	`, canceledAtValue, operationID); err != nil {
		return BackgroundOperationCancellation{}, fmt.Errorf("cancel background operation attempts: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE background_tasks
		SET status = 'canceled',
		    finished_at = ?,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    last_error_code = '',
		    last_error_message = ''
		WHERE operation_id = ? AND status IN ('pending', 'running')
	`, canceledAtValue, operationID); err != nil {
		return BackgroundOperationCancellation{}, fmt.Errorf("cancel background operation tasks: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE background_operations
		SET progress_completed = (
		        SELECT count(*) FROM background_tasks
		        WHERE operation_id = ? AND status = 'completed'
		    ),
		    progress_failed = (
		        SELECT count(*) FROM background_tasks
		        WHERE operation_id = ? AND status = 'failed'
		    )
		WHERE id = ?
	`, operationID, operationID, operationID); err != nil {
		return BackgroundOperationCancellation{}, fmt.Errorf("refresh canceled background operation progress: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BackgroundOperationCancellation{}, fmt.Errorf("commit background operation cancellation: %w", err)
	}
	return result, nil
}
