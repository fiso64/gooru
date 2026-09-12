package database

import "fmt"

// ListActiveBackgroundOperationIDs returns every pending/running operation ID.
// Unlike bounded history reads, this action-oriented query is intentionally
// unbounded so bulk cancellation cannot miss older active work behind terminal
// history rows.
func (s *Store) ListActiveBackgroundOperationIDs(visibleOnly bool) ([]string, error) {
	query := `
		SELECT id
		FROM background_operations
		WHERE status IN ('pending', 'running')
	`
	if visibleOnly {
		query += " AND visible = 1"
	}
	query += " ORDER BY created_at DESC, id DESC"

	rows, err := s.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list active background operation ids: %w", err)
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan active background operation id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active background operation ids: %w", err)
	}
	return ids, nil
}
