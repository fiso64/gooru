package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrBackgroundTaskLeaseLost = errors.New("background task lease is no longer owned by this worker")

// ClaimNextBackgroundTask atomically claims the highest-priority available pending task
// in one resource class, starts a durable attempt, and returns the claimed task. The
// single UPDATE statement is the scheduling race boundary: concurrent workers cannot
// both claim the same pending row.
func (s *Store) ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (BackgroundTask, bool, error) {
	if s == nil || s.DB == nil {
		return BackgroundTask{}, false, errors.New("background task store is required")
	}
	if resourceClass == "" {
		return BackgroundTask{}, false, errors.New("background task resource class is required")
	}
	if workerID == "" {
		return BackgroundTask{}, false, errors.New("background task worker id is required")
	}
	if leaseDuration <= 0 {
		return BackgroundTask{}, false, errors.New("background task lease duration must be positive")
	}
	now = normalizeWorkTime(now)
	leaseExpiresAt := now.Add(leaseDuration)

	tx, err := s.Begin()
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("begin background task claim: %w", err)
	}
	defer tx.Rollback()

	task, err := scanBackgroundTask(tx.QueryRow(`
		UPDATE background_tasks
		SET status = 'running',
		    lease_owner = ?,
		    lease_expires_at = ?,
		    started_at = COALESCE(started_at, ?),
		    attempt_count = attempt_count + 1
		WHERE id = (
			SELECT id
			FROM background_tasks
			WHERE resource_class = ?
			  AND status = 'pending'
			  AND available_at <= ?
			  AND attempt_count < max_attempts
			ORDER BY priority DESC, created_at, id
			LIMIT 1
		)
		  AND status = 'pending'
		RETURNING id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
		          resource_class, priority, status, available_at, lease_owner, lease_expires_at,
		          created_at, started_at, finished_at, attempt_count, max_attempts,
		          last_error_code, last_error_message
	`, workerID, workTimeValue(leaseExpiresAt), workTimeValue(now), resourceClass, workTimeValue(now)))
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundTask{}, false, nil
	}
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("claim background task: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO background_task_attempts (task_id, attempt_number, worker_id, started_at)
		VALUES (?, ?, ?, ?)
	`, task.ID, task.AttemptCount, workerID, workTimeValue(now)); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("record background task attempt: %w", err)
	}
	if err := refreshBackgroundOperation(tx, task.OperationID, now); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("refresh background operation after task claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("commit background task claim: %w", err)
	}
	return task, true, nil
}

// CompleteBackgroundTask completes a task and its currently running attempt iff workerID
// still owns a live task lease. A stale or expired worker receives
// ErrBackgroundTaskLeaseLost instead of overwriting recovery or a newer attempt.
func (s *Store) CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error {
	if s == nil || s.DB == nil {
		return errors.New("background task store is required")
	}
	if taskID == "" || workerID == "" {
		return errors.New("background task id and worker id are required")
	}
	finishedAt = normalizeWorkTime(finishedAt)

	tx, err := s.Begin()
	if err != nil {
		return fmt.Errorf("begin background task completion: %w", err)
	}
	defer tx.Rollback()

	var attemptNumber int
	var operationID sql.NullString
	if err := tx.QueryRow(`
		UPDATE background_tasks
		SET status = 'completed',
		    finished_at = ?,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    last_error_code = '',
		    last_error_message = ''
		WHERE id = ?
		  AND status = 'running'
		  AND lease_owner = ?
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at > ?
		RETURNING attempt_count, operation_id
	`, workTimeValue(finishedAt), taskID, workerID, workTimeValue(finishedAt)).Scan(&attemptNumber, &operationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrBackgroundTaskLeaseLost
		}
		return fmt.Errorf("complete background task: %w", err)
	}

	res, err := tx.Exec(`
		UPDATE background_task_attempts
		SET finished_at = ?, outcome = 'completed', error_code = '', error_message = ''
		WHERE task_id = ? AND attempt_number = ? AND worker_id = ? AND outcome = 'running'
	`, workTimeValue(finishedAt), taskID, attemptNumber, workerID)
	if err != nil {
		return fmt.Errorf("complete background task attempt: %w", err)
	}
	if err := requireOneBackgroundAttempt(res); err != nil {
		return fmt.Errorf("complete background task attempt: %w", err)
	}
	if err := refreshBackgroundOperation(tx, operationID.String, finishedAt); err != nil {
		return fmt.Errorf("refresh background operation after task completion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit background task completion: %w", err)
	}
	return nil
}

// FailBackgroundTask closes the current attempt as failed iff workerID still owns a live
// task lease. When retry capacity remains, the task returns to pending at retryAt;
// otherwise it becomes terminally failed.
func (s *Store) FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error) {
	if s == nil || s.DB == nil {
		return false, errors.New("background task store is required")
	}
	if taskID == "" || workerID == "" {
		return false, errors.New("background task id and worker id are required")
	}
	finishedAt = normalizeWorkTime(finishedAt)
	if retryAt.IsZero() {
		retryAt = finishedAt
	} else {
		retryAt = retryAt.UTC()
	}

	tx, err := s.Begin()
	if err != nil {
		return false, fmt.Errorf("begin background task failure: %w", err)
	}
	defer tx.Rollback()

	var attemptNumber, maxAttempts int
	var status string
	var operationID sql.NullString
	if err := tx.QueryRow(`
		UPDATE background_tasks
		SET status = CASE WHEN attempt_count < max_attempts THEN 'pending' ELSE 'failed' END,
		    available_at = CASE WHEN attempt_count < max_attempts THEN ? ELSE available_at END,
		    finished_at = CASE WHEN attempt_count < max_attempts THEN NULL ELSE ? END,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    last_error_code = ?,
		    last_error_message = ?
		WHERE id = ?
		  AND status = 'running'
		  AND lease_owner = ?
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at > ?
		RETURNING attempt_count, max_attempts, status, operation_id
	`, workTimeValue(retryAt), workTimeValue(finishedAt), errorCode, errorMessage, taskID, workerID, workTimeValue(finishedAt)).Scan(&attemptNumber, &maxAttempts, &status, &operationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, ErrBackgroundTaskLeaseLost
		}
		return false, fmt.Errorf("fail background task: %w", err)
	}

	res, err := tx.Exec(`
		UPDATE background_task_attempts
		SET finished_at = ?, outcome = 'failed', error_code = ?, error_message = ?
		WHERE task_id = ? AND attempt_number = ? AND worker_id = ? AND outcome = 'running'
	`, workTimeValue(finishedAt), errorCode, errorMessage, taskID, attemptNumber, workerID)
	if err != nil {
		return false, fmt.Errorf("fail background task attempt: %w", err)
	}
	if err := requireOneBackgroundAttempt(res); err != nil {
		return false, fmt.Errorf("fail background task attempt: %w", err)
	}
	if err := refreshBackgroundOperation(tx, operationID.String, finishedAt); err != nil {
		return false, fmt.Errorf("refresh background operation after task failure: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit background task failure: %w", err)
	}
	return BackgroundWorkStatus(status) == BackgroundWorkPending && attemptNumber < maxAttempts, nil
}

// RecoverExpiredBackgroundTaskLeases abandons attempts whose worker lease expired. Tasks
// with retry capacity return to pending immediately; exhausted tasks become failed. Every
// transition is guarded by the still-expired running state so a concurrent completion or
// lease replacement wins cleanly instead of being overwritten.
func (s *Store) RecoverExpiredBackgroundTaskLeases(now time.Time) (int, error) {
	if s == nil || s.DB == nil {
		return 0, errors.New("background task store is required")
	}
	now = normalizeWorkTime(now)
	nowValue := workTimeValue(now)

	tx, err := s.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin background task lease recovery: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT id, attempt_count, operation_id
		FROM background_tasks
		WHERE status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?
		ORDER BY lease_expires_at, id
	`, nowValue)
	if err != nil {
		return 0, fmt.Errorf("list expired background task leases: %w", err)
	}
	type expiredTask struct {
		id            string
		attemptNumber int
		operationID   sql.NullString
	}
	var expired []expiredTask
	for rows.Next() {
		var task expiredTask
		if err := rows.Scan(&task.id, &task.attemptNumber, &task.operationID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired background task lease: %w", err)
		}
		expired = append(expired, task)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate expired background task leases: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close expired background task leases: %w", err)
	}

	recovered := 0
	for _, task := range expired {
		var status string
		res := tx.QueryRow(`
			UPDATE background_tasks
			SET status = CASE WHEN attempt_count < max_attempts THEN 'pending' ELSE 'failed' END,
			    available_at = CASE WHEN attempt_count < max_attempts THEN ? ELSE available_at END,
			    finished_at = CASE WHEN attempt_count < max_attempts THEN NULL ELSE ? END,
			    lease_owner = '',
			    lease_expires_at = NULL,
			    last_error_code = 'lease_expired',
			    last_error_message = 'worker lease expired before task completion'
			WHERE id = ? AND status = 'running' AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?
			RETURNING status
		`, nowValue, nowValue, task.id, nowValue)
		if err := res.Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return 0, fmt.Errorf("recover expired background task %s: %w", task.id, err)
		}
		attemptResult, err := tx.Exec(`
			UPDATE background_task_attempts
			SET finished_at = ?, outcome = 'abandoned',
			    error_code = 'lease_expired',
			    error_message = 'worker lease expired before task completion'
			WHERE task_id = ? AND attempt_number = ? AND outcome = 'running'
		`, nowValue, task.id, task.attemptNumber)
		if err != nil {
			return 0, fmt.Errorf("abandon expired background task attempt %s/%d: %w", task.id, task.attemptNumber, err)
		}
		if err := requireOneBackgroundAttempt(attemptResult); err != nil {
			return 0, fmt.Errorf("abandon expired background task attempt %s/%d: %w", task.id, task.attemptNumber, err)
		}
		if err := refreshBackgroundOperation(tx, task.operationID.String, now); err != nil {
			return 0, fmt.Errorf("refresh background operation after expired task %s: %w", task.id, err)
		}
		recovered++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit background task lease recovery: %w", err)
	}
	return recovered, nil
}

func requireOneBackgroundAttempt(res sql.Result) error {
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("updated %d attempt rows, want 1", rows)
	}
	return nil
}
