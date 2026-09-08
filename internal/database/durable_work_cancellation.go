package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

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

// CancelBackgroundOperation durably cancels one active logical operation and all
// of its pending/running child tasks in the same transaction. Marking the parent
// canceled first makes cancellation sticky: the schema rejects any later child
// enqueue for a terminal operation. Existing completed/failed history is kept for
// aggregate diagnostics while active attempts are closed as canceled.
func (s *Store) CancelBackgroundOperation(operationID string, canceledAt time.Time) (canceled bool, err error) {
	if s == nil || s.DB == nil {
		return false, errors.New("background task store is required")
	}
	if operationID == "" {
		return false, errors.New("background operation id is required")
	}
	canceledAt = normalizeWorkTime(canceledAt)
	canceledAtValue := workTimeValue(canceledAt)

	tx, err := s.Begin()
	if err != nil {
		return false, fmt.Errorf("begin background operation cancellation: %w", err)
	}
	defer tx.Rollback()

	// Acquire the operation as an active row and make its terminal intent visible
	// to all later statements in this transaction. A concurrent enqueue is
	// serialized by SQLite and, after this commits, is rejected by the trigger.
	res, err := tx.Exec(`
		UPDATE background_operations
		SET status = 'canceled',
		    started_at = COALESCE(started_at, ?),
		    finished_at = ?,
		    error_code = '',
		    error_message = ''
		WHERE id = ? AND status IN ('pending', 'running')
	`, canceledAtValue, canceledAtValue, operationID)
	if err != nil {
		return false, fmt.Errorf("cancel background operation: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("cancel background operation rows affected: %w", err)
	}
	if rows == 0 {
		return false, nil
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
		return false, fmt.Errorf("cancel background operation attempts: %w", err)
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
		return false, fmt.Errorf("cancel background operation tasks: %w", err)
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
		return false, fmt.Errorf("refresh canceled background operation progress: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit background operation cancellation: %w", err)
	}
	return true, nil
}
