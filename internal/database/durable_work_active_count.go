package database

import "fmt"

// CountActiveBackgroundOperations returns the number of pending or running
// durable operations. When visibleOnly is true, hidden implementation work is
// excluded so UI badges reflect only user-visible Jobs activity.
func (s *Store) CountActiveBackgroundOperations(visibleOnly bool) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM background_operations
		WHERE status IN ('pending', 'running')
	`
	if visibleOnly {
		query += " AND visible = 1"
	}

	var count int
	if err := s.DB.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active background operations: %w", err)
	}
	return count, nil
}

// CountBackgroundOperations returns the exact number of durable operations.
// When visibleOnly is true, hidden implementation work is excluded.
func (s *Store) CountBackgroundOperations(visibleOnly bool) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM background_operations
	`
	if visibleOnly {
		query += " WHERE visible = 1"
	}

	var count int
	if err := s.DB.QueryRow(query).Scan(&count); err != nil {
		return 0, fmt.Errorf("count background operations: %w", err)
	}
	return count, nil
}
