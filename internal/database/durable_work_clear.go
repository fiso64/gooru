package database

import "fmt"

// ClearTerminalBackgroundOperations removes visible terminal operation history
// together with its attached terminal task history. Active attached or detached
// operation-scoped work is an additional safety barrier: even a corrupt/stale
// terminal operation row is kept while recovery or compensation is still active.
func (s *Store) ClearTerminalBackgroundOperations() (int64, error) {
	tx, err := s.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin clear terminal background operations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		DELETE FROM background_tasks
		WHERE operation_id IN (
			SELECT operation.id
			FROM background_operations AS operation
			WHERE operation.visible = 1
			  AND operation.status IN ('completed', 'failed', 'canceled')
			  AND NOT EXISTS (
				SELECT 1
				FROM background_tasks AS active
				WHERE active.operation_id = operation.id
				  AND active.status IN ('pending', 'running')
			  )
			  AND NOT EXISTS (
				SELECT 1
				FROM background_tasks AS cleanup
				WHERE cleanup.operation_id IS NULL
				  AND cleanup.subject_kind = 'operation'
				  AND cleanup.subject_id = operation.id
				  AND cleanup.status IN ('pending', 'running')
			  )
		)
	`)
	if err != nil {
		return 0, fmt.Errorf("clear terminal background task history: %w", err)
	}

	result, err := tx.Exec(`
		DELETE FROM background_operations AS operation
		WHERE operation.visible = 1
		  AND operation.status IN ('completed', 'failed', 'canceled')
		  AND NOT EXISTS (
			SELECT 1
			FROM background_tasks AS active
			WHERE active.operation_id = operation.id
			  AND active.status IN ('pending', 'running')
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM background_tasks AS cleanup
			WHERE cleanup.operation_id IS NULL
			  AND cleanup.subject_kind = 'operation'
			  AND cleanup.subject_id = operation.id
			  AND cleanup.status IN ('pending', 'running')
		  )
	`)
	if err != nil {
		return 0, fmt.Errorf("clear terminal background operation history: %w", err)
	}
	cleared, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cleared background operation count: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit clear terminal background operations: %w", err)
	}
	return cleared, nil
}
