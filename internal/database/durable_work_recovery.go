package database

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// CancelUnattachedHiddenBackgroundOperations releases durable admission
// reservations left behind by a process death before the producer attached its
// child task. It is intended for startup reconciliation, before new requests can
// create in-flight hidden reservations in the current process.
func (s *Store) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	if s == nil || s.DB == nil {
		return 0, errors.New("background operation store is required")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return 0, errors.New("background operation kind is required")
	}
	finishedAt := workTimeValue(time.Now().UTC())
	result, err := s.DB.Exec(`
		UPDATE background_operations
		SET status = 'canceled', finished_at = ?,
		    error_code = 'producer_interrupted',
		    error_message = 'producer interrupted before durable work was attached'
		WHERE kind = ?
		  AND visible = 0
		  AND status = 'pending'
		  AND NOT EXISTS (
			SELECT 1 FROM background_tasks
			WHERE background_tasks.operation_id = background_operations.id
		  )
	`, finishedAt, kind)
	if err != nil {
		return 0, fmt.Errorf("cancel unattached hidden background operations: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("cancel unattached hidden background operations rows affected: %w", err)
	}
	return count, nil
}
