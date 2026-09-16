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

// ListUnattachedBackgroundOperationIDs returns pending operations of one kind
// that have no durable child task without mutating them. Producers with external
// recovery state use this to reclaim that state before making interruption
// cancellation terminal, so cleanup failures remain discoverable on restart.
func (s *Store) ListUnattachedBackgroundOperationIDs(kind string) ([]string, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("background operation store is required")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, errors.New("background operation kind is required")
	}
	rows, err := s.DB.Query(`
		SELECT id
		FROM background_operations
		WHERE kind = ?
		  AND status = 'pending'
		  AND NOT EXISTS (
			SELECT 1 FROM background_tasks
			WHERE background_tasks.operation_id = background_operations.id
		  )
		ORDER BY id
	`, kind)
	if err != nil {
		return nil, fmt.Errorf("list unattached background operations: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read unattached background operation id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list unattached background operations rows: %w", err)
	}
	return ids, nil
}

// CancelUnattachedBackgroundOperations cancels pending operations of one kind
// that have no durable child task, regardless of visibility. Visible upload
// receiving operations use this at startup because a process death can occur
// after publication but before multipart staging attaches durable work.
func (s *Store) CancelUnattachedBackgroundOperations(kind string) (int64, error) {
	ids, err := s.CancelUnattachedBackgroundOperationIDs(kind)
	return int64(len(ids)), err
}

// CancelUnattachedBackgroundOperationIDs is the startup reconciliation variant
// for producers that own external staging state. It returns exactly the durable
// operation ids canceled by this statement so callers can reclaim only state
// owned by those interrupted producers.
func (s *Store) CancelUnattachedBackgroundOperationIDs(kind string) ([]string, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("background operation store is required")
	}
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, errors.New("background operation kind is required")
	}
	finishedAt := workTimeValue(time.Now().UTC())
	rows, err := s.DB.Query(`
		UPDATE background_operations
		SET status = 'canceled', finished_at = ?,
		    error_code = 'producer_interrupted',
		    error_message = 'producer interrupted before durable work was attached'
		WHERE kind = ?
		  AND status = 'pending'
		  AND NOT EXISTS (
			SELECT 1 FROM background_tasks
			WHERE background_tasks.operation_id = background_operations.id
		  )
		RETURNING id
	`, finishedAt, kind)
	if err != nil {
		return nil, fmt.Errorf("cancel unattached background operations: %w", err)
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("read canceled unattached background operation id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cancel unattached background operations rows: %w", err)
	}
	return ids, nil
}
