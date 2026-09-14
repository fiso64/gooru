package database

import "errors"

// ClaimBackgroundOperationProducer atomically grants one producer the right
// to stage and attach the first child task for an admitted operation.
func (s *Store) ClaimBackgroundOperationProducer(operationID string) (bool, error) {
	if s == nil || s.DB == nil {
		return false, errors.New("background operation store is required")
	}
	if operationID == "" {
		return false, errors.New("background operation id is required")
	}
	result, err := s.DB.Exec(`
		UPDATE background_operations
		SET producer_claimed = 1
		WHERE id = ?
		  AND status = 'pending'
		  AND producer_claimed = 0
		  AND NOT EXISTS (
			SELECT 1 FROM background_tasks
			WHERE operation_id = background_operations.id
		  )
	`, operationID)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return changed == 1, nil
}
