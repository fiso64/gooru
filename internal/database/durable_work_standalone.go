package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// FindActiveStandaloneBackgroundOperationIDByKind returns the newest pending/running
// operation with the requested kind that is not associated with a producer. Ordinary
// ReuseActive work must not inherit the producer-coupled lifetime of an auxiliary
// operation merely because both operations share a kind.
func (s *Store) FindActiveStandaloneBackgroundOperationIDByKind(q Querier, kind string) (string, bool, error) {
	if q == nil {
		return "", false, errors.New("background operation querier is required")
	}
	if kind == "" {
		return "", false, errors.New("background operation kind is required")
	}
	var id string
	err := q.QueryRow(`
		SELECT operations.id
		FROM background_operations operations
		WHERE operations.kind = ?
		  AND operations.status IN ('pending', 'running')
		  AND NOT EXISTS (
			SELECT 1
			FROM background_operation_associations associations
			WHERE associations.auxiliary_operation_id = operations.id
		  )
		ORDER BY operations.created_at DESC, operations.id DESC
		LIMIT 1
	`, kind).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find active standalone background operation by kind: %w", err)
	}
	return id, true, nil
}
