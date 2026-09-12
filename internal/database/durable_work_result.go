package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const maxBackgroundOperationResultBytes = 1 << 20

// SetBackgroundOperationResult persists the structured success result for an active
// logical operation. Results are deliberately bounded so a producer cannot turn the
// operation history table into an unbounded blob store. The result is only exposed to
// consumers after the operation reaches completed state.
func (s *Store) SetBackgroundOperationResult(operationID string, resultJSON []byte) error {
	if s == nil || s.DB == nil {
		return errors.New("background operation store is required")
	}
	return s.setBackgroundOperationResult(s.DB, operationID, resultJSON)
}

// SetBackgroundOperationResultTx persists a success payload inside an
// existing domain transaction so consumers can never observe a completed
// mutation whose durable replay result was lost in a separate commit.
func (s *Store) SetBackgroundOperationResultTx(tx *Tx, operationID string, resultJSON []byte) error {
	if s == nil {
		return errors.New("background operation store is required")
	}
	return s.setBackgroundOperationResult(tx, operationID, resultJSON)
}

func (s *Store) setBackgroundOperationResult(q Querier, operationID string, resultJSON []byte) error {
	if q == nil {
		return errors.New("background operation querier is required")
	}
	if operationID == "" {
		return errors.New("background operation id is required")
	}
	if len(resultJSON) == 0 || !json.Valid(resultJSON) {
		return errors.New("background operation result must be valid JSON")
	}
	if len(resultJSON) > maxBackgroundOperationResultBytes {
		return fmt.Errorf("background operation result exceeds %d bytes", maxBackgroundOperationResultBytes)
	}
	res, err := q.Exec(`
		UPDATE background_operations
		SET result_json = ?
		WHERE id = ? AND status IN ('pending', 'running')
	`, string(resultJSON), operationID)
	if err != nil {
		return fmt.Errorf("set background operation result: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background operation result rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("background operation is missing or no longer active")
	}
	return nil
}

// GetBackgroundOperationResult returns a completed operation's structured result. An
// active, failed, canceled, missing, or result-less operation returns found=false so
// callers cannot observe a success payload before the lifecycle says it is complete.
func (s *Store) GetBackgroundOperationResult(operationID string) ([]byte, bool, error) {
	if s == nil || s.DB == nil {
		return nil, false, errors.New("background operation store is required")
	}
	if operationID == "" {
		return nil, false, errors.New("background operation id is required")
	}
	var result sql.NullString
	if err := s.DB.QueryRow(`
		SELECT result_json
		FROM background_operations
		WHERE id = ? AND status = 'completed' AND COALESCE(result_json, '') <> ''
	`, operationID).Scan(&result); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read background operation result: %w", err)
	}
	if !result.Valid || result.String == "" {
		return nil, false, nil
	}
	return []byte(result.String), true, nil
}
