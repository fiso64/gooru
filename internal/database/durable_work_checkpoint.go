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
	return s.setBackgroundOperationCheckpoint(s.DB, operationID, checkpointJSON)
}

// SetBackgroundOperationCheckpointTx persists recovery state inside an
// existing domain transaction so the checkpoint cannot diverge from the
// mutation it describes.
func (s *Store) SetBackgroundOperationCheckpointTx(tx *Tx, operationID string, checkpointJSON []byte) error {
	if s == nil {
		return errors.New("background operation store is required")
	}
	return s.setBackgroundOperationCheckpoint(tx, operationID, checkpointJSON)
}

func (s *Store) setBackgroundOperationCheckpoint(q Querier, operationID string, checkpointJSON []byte) error {
	if q == nil {
		return errors.New("background operation querier is required")
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
	res, err := q.Exec(`
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

// ListActiveBackgroundOperationCheckpoints returns producer-owned recovery state
// for every active operation of one kind. This intentionally has no UI-style
// history limit: startup recovery must not forget work merely because a queue is
// large.
func (s *Store) ListActiveBackgroundOperationCheckpoints(kind string) (map[string][]byte, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("background operation store is required")
	}
	if kind == "" {
		return nil, errors.New("background operation kind is required")
	}
	rows, err := s.DB.Query(`
		SELECT id, checkpoint_json
		FROM background_operations
		WHERE kind = ?
		  AND status IN ('pending', 'running')
		  AND COALESCE(checkpoint_json, '') <> ''
	`, kind)
	if err != nil {
		return nil, fmt.Errorf("list active background operation checkpoints: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]byte)
	for rows.Next() {
		var operationID string
		var checkpoint string
		if err := rows.Scan(&operationID, &checkpoint); err != nil {
			return nil, fmt.Errorf("scan active background operation checkpoint: %w", err)
		}
		result[operationID] = []byte(checkpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active background operation checkpoints: %w", err)
	}
	return result, nil
}

// SetBackgroundOperationVisible changes whether an operation is exposed through
// user-facing operation history.
func (s *Store) SetBackgroundOperationVisible(operationID string, visible bool) error {
	if s == nil || s.DB == nil {
		return errors.New("background operation store is required")
	}
	return s.setBackgroundOperationVisible(s.DB, operationID, visible)
}

func (s *Store) setBackgroundOperationVisible(q Querier, operationID string, visible bool) error {
	if q == nil {
		return errors.New("background operation querier is required")
	}
	value := 0
	if visible {
		value = 1
	}
	res, err := q.Exec(`UPDATE background_operations SET visible = ? WHERE id = ?`, value, operationID)
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
