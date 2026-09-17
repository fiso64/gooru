package database

import (
	"database/sql"
	"fmt"
	"time"
)

// markBackgroundOperationStarted records the only aggregate state change caused
// by a task claim. A running child proves the operation is running, so there is
// no need to rescan every sibling task just to derive that fact.
func markBackgroundOperationStarted(tx *Tx, operationID string, now time.Time) error {
	if operationID == "" {
		return nil
	}
	now = normalizeWorkTime(now)
	res, err := tx.Exec(`
		UPDATE background_operations
		SET status = 'running',
		    started_at = COALESCE(started_at, ?),
		    finished_at = NULL,
		    error_code = '',
		    error_message = '',
		    result_json = CASE
		        WHEN status IN ('completed', 'failed') THEN ''
		        ELSE result_json
		    END
		WHERE id = ?
	`, workTimeValue(now), operationID)
	if err != nil {
		return fmt.Errorf("mark background operation %s running: %w", operationID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark background operation %s running rows affected: %w", operationID, err)
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// recordBackgroundOperationTaskTerminal applies the exact counter delta from a
// guarded child-task transition. The task row and these counters are changed in
// the same transaction, so retries or stale workers cannot double-count them.
//
// Most transitions stop after an indexed existence check for remaining active
// children. Only the operation's final active child needs a canceled-child count
// to choose the terminal status. This keeps large fan-out operations linear in
// the number of task transitions instead of rescanning every sibling on every
// claim and completion.
func recordBackgroundOperationTaskTerminal(tx *Tx, operationID string, now time.Time, completedDelta, failedDelta int64) error {
	if operationID == "" {
		return nil
	}
	now = normalizeWorkTime(now)

	var progressTotal, completed, failed int64
	if err := tx.QueryRow(`
		UPDATE background_operations
		SET progress_completed = progress_completed + ?,
		    progress_failed = progress_failed + ?
		WHERE id = ?
		RETURNING progress_total, progress_completed, progress_failed
	`, completedDelta, failedDelta, operationID).Scan(&progressTotal, &completed, &failed); err != nil {
		return fmt.Errorf("advance background operation %s progress: %w", operationID, err)
	}

	var active int
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM background_tasks
			WHERE operation_id = ? AND status IN ('pending', 'running')
			LIMIT 1
		)
	`, operationID).Scan(&active); err != nil {
		return fmt.Errorf("inspect active background tasks for operation %s: %w", operationID, err)
	}
	if active != 0 {
		return nil
	}

	var canceled int64
	if err := tx.QueryRow(`
		SELECT count(*)
		FROM background_tasks
		WHERE operation_id = ? AND status = 'canceled'
	`, operationID).Scan(&canceled); err != nil {
		return fmt.Errorf("count canceled background tasks for operation %s: %w", operationID, err)
	}

	terminalCount := completed + failed + canceled
	if progressTotal > 0 && terminalCount < progressTotal {
		// The operation declared more work than is attached today. Preserve its
		// current pending/running state until later child attachment supplies the
		// remaining work, matching the previous aggregate-derived semantics.
		return nil
	}

	var producerActive int
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM background_operation_associations AS association
			JOIN background_operations AS producer
			  ON producer.id = association.producer_operation_id
			WHERE association.auxiliary_operation_id = ?
			  AND producer.status IN ('pending', 'running')
			LIMIT 1
		)
	`, operationID).Scan(&producerActive); err != nil {
		return fmt.Errorf("inspect active producer for background operation %s: %w", operationID, err)
	}
	if producerActive != 0 {
		// Associated auxiliary operations remain active while their producer can
		// still commit more work. The association extends only the auxiliary
		// lifetime and never consumes producer progress slots.
		return nil
	}

	status := BackgroundWorkCompleted
	errorCode := ""
	errorMessage := ""
	switch {
	case failed > 0:
		status = BackgroundWorkFailed
		errorCode = "child_task_failed"
		errorMessage = fmt.Sprintf("%d background task(s) failed", failed)
	case canceled > 0:
		status = BackgroundWorkCanceled
	}

	res, err := tx.Exec(`
		UPDATE background_operations
		SET status = ?,
		    started_at = COALESCE(started_at, ?),
		    finished_at = ?,
		    error_code = ?,
		    error_message = ?
		WHERE id = ?
	`, status, workTimeValue(now), workTimeValue(now), errorCode, errorMessage, operationID)
	if err != nil {
		return fmt.Errorf("finish background operation %s: %w", operationID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish background operation %s rows affected: %w", operationID, err)
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}
