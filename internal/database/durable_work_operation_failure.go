package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BackgroundOperationFailure describes the ownership state at the exact
// transaction where a producer-side operation failure won. RunningTasks lets
// callers delay compensation until workers whose leases were revoked have had
// time to observe ownership loss.
type BackgroundOperationFailure struct {
	Failed       bool
	RunningTasks int64
}

// FailBackgroundOperation marks an active logical operation failed and cancels
// any still-active child work. Producers that own external side effects should
// use FailBackgroundOperationWithCleanupTask so cleanup is crash-safe.
func (s *Store) FailBackgroundOperation(operationID string, failedAt time.Time, errorCode, errorMessage string) (bool, error) {
	result, err := s.failBackgroundOperation(operationID, failedAt, errorCode, errorMessage, nil)
	return result.Failed, err
}

// FailBackgroundOperationWithCleanupTask atomically records producer failure,
// revokes pending/running child work, and (when child work exists) creates one
// detached cleanup obligation. Cleanup is delayed through the latest revoked
// worker lease so compensation cannot race an old worker still unwinding.
func (s *Store) FailBackgroundOperationWithCleanupTask(operationID string, failedAt time.Time, errorCode, errorMessage string, cleanup NewBackgroundTask) (BackgroundOperationFailure, error) {
	if cleanup.OperationID != "" {
		return BackgroundOperationFailure{}, errors.New("background failure cleanup task must be detached from an operation")
	}
	return s.failBackgroundOperation(operationID, failedAt, errorCode, errorMessage, &cleanup)
}

func (s *Store) failBackgroundOperation(operationID string, failedAt time.Time, errorCode, errorMessage string, cleanup *NewBackgroundTask) (result BackgroundOperationFailure, err error) {
	if s == nil || s.DB == nil {
		return result, errors.New("background task store is required")
	}
	if operationID == "" {
		return result, errors.New("background operation id is required")
	}
	failedAt = normalizeWorkTime(failedAt)
	failedAtValue := workTimeValue(failedAt)

	tx, err := s.Begin()
	if err != nil {
		return result, fmt.Errorf("begin background operation failure: %w", err)
	}
	defer tx.Rollback()

	// A persisted producer result is the same commit boundary used by
	// cancellation: once durable success exists, a concurrent producer failure is
	// too late to replace it.
	res, err := tx.Exec(`
		UPDATE background_operations
		SET status = 'failed',
		    started_at = COALESCE(started_at, ?),
		    finished_at = ?,
		    error_code = ?,
		    error_message = ?
		WHERE id = ?
		  AND status IN ('pending', 'running')
		  AND COALESCE(result_json, '') = ''
	`, failedAtValue, failedAtValue, errorCode, errorMessage, operationID)
	if err != nil {
		return result, fmt.Errorf("fail background operation: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return result, fmt.Errorf("fail background operation rows affected: %w", err)
	}
	if rows == 0 {
		return result, nil
	}
	result.Failed = true

	var childTasks int64
	var latestRunningLease sql.NullInt64
	if err := tx.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END), 0),
		       MAX(CASE WHEN status = 'running' THEN lease_expires_at END)
		FROM background_tasks
		WHERE operation_id = ?
	`, operationID).Scan(&childTasks, &result.RunningTasks, &latestRunningLease); err != nil {
		return BackgroundOperationFailure{}, fmt.Errorf("inspect background operation tasks during failure: %w", err)
	}
	if result.RunningTasks > 0 && !latestRunningLease.Valid {
		return BackgroundOperationFailure{}, errors.New("running background task is missing a lease expiry")
	}

	if _, err := tx.Exec(`
		UPDATE background_task_attempts
		SET finished_at = ?, outcome = 'canceled', error_code = ?, error_message = ?
		WHERE outcome = 'running'
		  AND task_id IN (
			SELECT id FROM background_tasks
			WHERE operation_id = ? AND status = 'running'
		  )
	`, failedAtValue, errorCode, errorMessage, operationID); err != nil {
		return BackgroundOperationFailure{}, fmt.Errorf("cancel background operation attempts after producer failure: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE background_tasks
		SET status = 'canceled',
		    finished_at = ?,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    last_error_code = ?,
		    last_error_message = ?
		WHERE operation_id = ? AND status IN ('pending', 'running')
	`, failedAtValue, errorCode, errorMessage, operationID); err != nil {
		return BackgroundOperationFailure{}, fmt.Errorf("cancel background operation tasks after producer failure: %w", err)
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
		return BackgroundOperationFailure{}, fmt.Errorf("refresh failed background operation progress: %w", err)
	}

	if cleanup != nil && childTasks > 0 {
		cleanupTask := *cleanup
		if cleanupTask.CreatedAt.IsZero() {
			cleanupTask.CreatedAt = failedAt
		}
		safeAt := failedAt
		if latestRunningLease.Valid {
			leaseAt := workTime(latestRunningLease.Int64)
			if leaseAt.After(safeAt) {
				safeAt = leaseAt
			}
		}
		if cleanupTask.AvailableAt.IsZero() || cleanupTask.AvailableAt.Before(safeAt) {
			cleanupTask.AvailableAt = safeAt
		}
		_, created, err := s.EnqueueBackgroundTask(tx, cleanupTask)
		if err != nil {
			return BackgroundOperationFailure{}, fmt.Errorf("enqueue background failure cleanup: %w", err)
		}
		if !created {
			return BackgroundOperationFailure{}, errors.New("background failure cleanup task dedupe key is already active")
		}
	}

	if err := tx.Commit(); err != nil {
		return BackgroundOperationFailure{}, fmt.Errorf("commit background operation failure: %w", err)
	}
	return result, nil
}
