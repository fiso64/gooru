package database

import (
	"database/sql"
	"fmt"
	"time"
)

// refreshBackgroundOperation derives one logical operation's lifecycle from its
// child tasks inside the same transaction that changed a task. This keeps the
// user-visible aggregate crash-consistent with durable scheduling state.
func refreshBackgroundOperation(tx *Tx, operationID string, now time.Time) error {
	if operationID == "" {
		return nil
	}
	now = normalizeWorkTime(now)

	var total, completed, failed, canceled, started int64
	if err := tx.QueryRow(`
		SELECT count(*),
		       COALESCE(sum(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
		       COALESCE(sum(CASE WHEN status = 'failed' THEN 1 ELSE 0 END), 0),
		       COALESCE(sum(CASE WHEN status = 'canceled' THEN 1 ELSE 0 END), 0),
		       COALESCE(sum(CASE WHEN started_at IS NOT NULL THEN 1 ELSE 0 END), 0)
		FROM background_tasks
		WHERE operation_id = ?
	`, operationID).Scan(&total, &completed, &failed, &canceled, &started); err != nil {
		return fmt.Errorf("aggregate background operation %s: %w", operationID, err)
	}
	if total == 0 {
		return nil
	}

	terminal := completed+failed+canceled == total
	status := BackgroundWorkPending
	if terminal {
		switch {
		case failed > 0:
			status = BackgroundWorkFailed
		case canceled > 0:
			status = BackgroundWorkCanceled
		default:
			status = BackgroundWorkCompleted
		}
	} else if started > 0 {
		status = BackgroundWorkRunning
	}

	var startedAt, finishedAt interface{}
	if started > 0 || terminal {
		startedAt = workTimeValue(now)
	}
	if terminal {
		finishedAt = workTimeValue(now)
	}

	errorCode := ""
	errorMessage := ""
	if status == BackgroundWorkFailed {
		errorCode = "child_task_failed"
		errorMessage = fmt.Sprintf("%d background task(s) failed", failed)
	}

	res, err := tx.Exec(`
		UPDATE background_operations
		SET status = ?,
		    progress_completed = ?,
		    progress_failed = ?,
		    started_at = CASE
		        WHEN started_at IS NULL AND ? IS NOT NULL THEN ?
		        ELSE started_at
		    END,
		    finished_at = ?,
		    error_code = ?,
		    error_message = ?
		WHERE id = ?
	`, status, completed, failed, startedAt, startedAt, finishedAt, errorCode, errorMessage, operationID)
	if err != nil {
		return fmt.Errorf("refresh background operation %s: %w", operationID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("refresh background operation %s rows affected: %w", operationID, err)
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}
