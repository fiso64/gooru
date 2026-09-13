package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	maxBackgroundTaskCheckpointBytes = 1 << 20
	maxBackgroundTaskResultBytes     = 1 << 20
)

// SetBackgroundTaskCheckpoint persists bounded producer recovery state for one
// active schedulable task. Task-scoped checkpoints let sibling children of one
// logical operation recover independently instead of overwriting shared state.
func (s *Store) SetBackgroundTaskCheckpoint(taskID string, checkpointJSON []byte) error {
	if s == nil || s.DB == nil {
		return errors.New("background task store is required")
	}
	return s.setBackgroundTaskCheckpoint(s.DB, taskID, checkpointJSON)
}

// SetBackgroundTaskCheckpointTx persists task recovery state inside an existing
// transaction so the checkpoint cannot diverge from the mutation it describes.
func (s *Store) SetBackgroundTaskCheckpointTx(tx *Tx, taskID string, checkpointJSON []byte) error {
	if s == nil {
		return errors.New("background task store is required")
	}
	return s.setBackgroundTaskCheckpoint(tx, taskID, checkpointJSON)
}

func (s *Store) setBackgroundTaskCheckpoint(q Querier, taskID string, checkpointJSON []byte) error {
	if q == nil {
		return errors.New("background task querier is required")
	}
	if taskID == "" {
		return errors.New("background task id is required")
	}
	if len(checkpointJSON) == 0 || !json.Valid(checkpointJSON) {
		return errors.New("background task checkpoint must be valid JSON")
	}
	if len(checkpointJSON) > maxBackgroundTaskCheckpointBytes {
		return fmt.Errorf("background task checkpoint exceeds %d bytes", maxBackgroundTaskCheckpointBytes)
	}
	res, err := q.Exec(`
		UPDATE background_tasks
		SET checkpoint_json = ?
		WHERE id = ? AND status IN ('pending', 'running')
	`, string(checkpointJSON), taskID)
	if err != nil {
		return fmt.Errorf("set background task checkpoint: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background task checkpoint rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("background task is missing or no longer active")
	}
	return nil
}

// GetBackgroundTaskCheckpoint loads task-owned recovery state regardless of the
// current lifecycle status. Terminal cleanup needs the final durable checkpoint
// after cancellation or failure has already made the task non-active.
func (s *Store) GetBackgroundTaskCheckpoint(taskID string) ([]byte, bool, error) {
	if s == nil || s.DB == nil {
		return nil, false, errors.New("background task store is required")
	}
	if taskID == "" {
		return nil, false, errors.New("background task id is required")
	}
	var checkpoint sql.NullString
	if err := s.DB.QueryRow(`
		SELECT checkpoint_json
		FROM background_tasks
		WHERE id = ? AND COALESCE(checkpoint_json, '') <> ''
	`, taskID).Scan(&checkpoint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read background task checkpoint: %w", err)
	}
	return []byte(checkpoint.String), checkpoint.Valid && checkpoint.String != "", nil
}

// SetBackgroundTaskResult persists a bounded structured success result while a
// task is active. Consumers only receive it after the task reaches completed.
func (s *Store) SetBackgroundTaskResult(taskID string, resultJSON []byte) error {
	if s == nil || s.DB == nil {
		return errors.New("background task store is required")
	}
	return s.setBackgroundTaskResult(s.DB, taskID, resultJSON)
}

// SetBackgroundTaskResultTx persists a task result inside an existing domain
// transaction so the replay payload cannot diverge from its mutation.
func (s *Store) SetBackgroundTaskResultTx(tx *Tx, taskID string, resultJSON []byte) error {
	if s == nil {
		return errors.New("background task store is required")
	}
	return s.setBackgroundTaskResult(tx, taskID, resultJSON)
}

func (s *Store) setBackgroundTaskResult(q Querier, taskID string, resultJSON []byte) error {
	if q == nil {
		return errors.New("background task querier is required")
	}
	if taskID == "" {
		return errors.New("background task id is required")
	}
	if len(resultJSON) == 0 || !json.Valid(resultJSON) {
		return errors.New("background task result must be valid JSON")
	}
	if len(resultJSON) > maxBackgroundTaskResultBytes {
		return fmt.Errorf("background task result exceeds %d bytes", maxBackgroundTaskResultBytes)
	}
	res, err := q.Exec(`
		UPDATE background_tasks
		SET result_json = ?
		WHERE id = ? AND status IN ('pending', 'running')
	`, string(resultJSON), taskID)
	if err != nil {
		return fmt.Errorf("set background task result: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set background task result rows affected: %w", err)
	}
	if rows != 1 {
		return errors.New("background task is missing or no longer active")
	}
	return nil
}

// GetBackgroundTaskResult returns a completed task's structured result. Active,
// failed, canceled, missing, and result-less tasks intentionally return found=false.
func (s *Store) GetBackgroundTaskResult(taskID string) ([]byte, bool, error) {
	if s == nil || s.DB == nil {
		return nil, false, errors.New("background task store is required")
	}
	if taskID == "" {
		return nil, false, errors.New("background task id is required")
	}
	var result sql.NullString
	if err := s.DB.QueryRow(`
		SELECT result_json
		FROM background_tasks
		WHERE id = ? AND status = 'completed' AND COALESCE(result_json, '') <> ''
	`, taskID).Scan(&result); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read background task result: %w", err)
	}
	if !result.Valid || result.String == "" {
		return nil, false, nil
	}
	return []byte(result.String), true, nil
}
