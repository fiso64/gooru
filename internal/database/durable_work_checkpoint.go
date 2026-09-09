package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const maxBackgroundOperationCheckpointBytes = 1 << 20

// SetBackgroundOperationCheckpoint persists bounded internal recovery state for
// an active logical operation. Unlike result_json, checkpoints are intentionally
// not exposed through the public operation API.
func (s *Store) SetBackgroundOperationCheckpoint(operationID string, checkpointJSON []byte) error {
	if s == nil || s.DB == nil {
		return errors.New("background operation store is required")
	}
	if operationID == "" {
		return errors.New("background operation id is required")
	}
	if len(checkpointJSON) == 0 || !json.Valid(checkpointJSON) {
		return errors.New("background operation checkpoint must be valid JSON")
	}
	if len(checkpointJSON) > maxBackgroundOperationCheckpointBytes {
		return fmt.Errorf("background operation checkpoint exceeds %d bytes", maxBackgroundOperationCheckpointBytes)
	}
	res, err := s.DB.Exec(`
		UPDATE background_operations
		SET checkpoint_json = ?
		WHERE id = ? AND status IN ('pending', 'running')
	`, string(checkpointJSON), operationID)
	if err != nil {
		return fmt.Errorf("set background operation checkpoint: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background operation checkpoint rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("background operation is missing or no longer active")
	}
	return nil
}

func (s *Store) GetBackgroundOperationCheckpoint(operationID string) ([]byte, bool, error) {
	if s == nil || s.DB == nil {
		return nil, false, errors.New("background operation store is required")
	}
	if operationID == "" {
		return nil, false, errors.New("background operation id is required")
	}
	var checkpoint sql.NullString
	if err := s.DB.QueryRow(`
		SELECT checkpoint_json
		FROM background_operations
		WHERE id = ? AND COALESCE(checkpoint_json, '') <> ''
	`, operationID).Scan(&checkpoint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read background operation checkpoint: %w", err)
	}
	return []byte(checkpoint.String), checkpoint.Valid && checkpoint.String != "", nil
}

// SetBackgroundOperationVisible reveals a previously hidden admitted operation
// after its durable child task has been attached successfully.
func (s *Store) SetBackgroundOperationVisible(operationID string, visible bool) error {
	if s == nil || s.DB == nil {
		return errors.New("background operation store is required")
	}
	value := 0
	if visible {
		value = 1
	}
	res, err := s.DB.Exec(`UPDATE background_operations SET visible = ? WHERE id = ?`, value, operationID)
	if err != nil {
		return fmt.Errorf("set background operation visibility: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background operation visibility rows affected: %w", err)
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}
