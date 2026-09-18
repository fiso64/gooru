package database

import (
	"errors"
	"fmt"
	"time"
)

// CreateBackgroundOperationWithPendingLimit atomically creates a pending
// operation only while fewer than maxPending operations of the same kind are
// already pending. The limit check and insert are one SQLite statement so
// concurrent producers cannot over-admit through a read-before-write race.
func (s *Store) CreateBackgroundOperationWithPendingLimit(id string, kind string, visible bool, progressTotal int64, maxPending int) (BackgroundOperation, bool, error) {
	return s.createBackgroundOperationWithPendingLimit(s, id, kind, visible, progressTotal, maxPending)
}

func (s *Store) createBackgroundOperationWithPendingLimit(q Querier, id string, kind string, visible bool, progressTotal int64, maxPending int) (BackgroundOperation, bool, error) {
	if q == nil {
		return BackgroundOperation{}, false, errors.New("background operation querier is required")
	}
	if id == "" {
		return BackgroundOperation{}, false, errors.New("background operation id is required")
	}
	if kind == "" {
		return BackgroundOperation{}, false, errors.New("background operation kind is required")
	}
	if progressTotal < 0 {
		return BackgroundOperation{}, false, errors.New("background operation progress total cannot be negative")
	}
	if maxPending < 1 {
		return BackgroundOperation{}, false, errors.New("background operation pending limit must be positive")
	}

	createdAt := normalizeWorkTime(time.Time{})
	visibleValue := 0
	if visible {
		visibleValue = 1
	}
	result, err := q.Exec(`
		INSERT INTO background_operations
			(id, kind, visible, status, progress_total, created_at)
		SELECT ?, ?, ?, 'pending', ?, ?
		WHERE (
			SELECT COUNT(*)
			FROM background_operations
			WHERE kind = ? AND status = 'pending'
		) < ?
	`, id, kind, visibleValue, progressTotal, workTimeValue(createdAt), kind, maxPending)
	if err != nil {
		return BackgroundOperation{}, false, fmt.Errorf("create bounded background operation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return BackgroundOperation{}, false, fmt.Errorf("create bounded background operation rows affected: %w", err)
	}
	if rows == 0 {
		return BackgroundOperation{}, false, nil
	}
	return BackgroundOperation{
		ID:            id,
		Kind:          kind,
		Visible:       visible,
		Status:        BackgroundWorkPending,
		ProgressTotal: progressTotal,
		CreatedAt:     createdAt,
	}, true, nil
}
