package database

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RenewBackgroundTaskLease extends a still-valid lease owned by workerID. Once a lease
// has expired, the worker must treat it as lost even if recovery has not yet run; this
// prevents a stale worker from reviving ownership while another worker may be recovering
// the task.
func (s *Store) RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error) {
	if s == nil || s.DB == nil {
		return time.Time{}, errors.New("background task store is required")
	}
	if taskID == "" || workerID == "" {
		return time.Time{}, errors.New("background task id and worker id are required")
	}
	if leaseDuration <= 0 {
		return time.Time{}, errors.New("background task lease duration must be positive")
	}
	now = normalizeWorkTime(now)
	expiresAt := now.Add(leaseDuration)

	var persisted int64
	if err := s.QueryRow(`
		UPDATE background_tasks
		SET lease_expires_at = ?
		WHERE id = ?
		  AND status = 'running'
		  AND lease_owner = ?
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at > ?
		RETURNING lease_expires_at
	`, workTimeValue(expiresAt), taskID, workerID, workTimeValue(now)).Scan(&persisted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, ErrBackgroundTaskLeaseLost
		}
		return time.Time{}, fmt.Errorf("renew background task lease: %w", err)
	}
	return workTime(persisted), nil
}
