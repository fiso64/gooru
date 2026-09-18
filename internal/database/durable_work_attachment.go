package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// AttachBackgroundTaskAndRevealOperation atomically persists producer recovery
// state, attaches the first durable child task to an admitted operation, and
// ensures the operation is visible. Most producers attach while hidden; upload
// operations may already be visible while their HTTP body is being received.
func (s *Store) AttachBackgroundTaskAndRevealOperation(operationID string, checkpointJSON []byte, task NewBackgroundTask) (BackgroundTask, error) {
	if s == nil || s.DB == nil {
		return BackgroundTask{}, errors.New("background operation store is required")
	}
	tx, err := s.Begin()
	if err != nil {
		return BackgroundTask{}, fmt.Errorf("begin background operation attachment: %w", err)
	}
	defer tx.Rollback()

	attachedTask, err := s.attachBackgroundTaskAndRevealOperation(tx, operationID, checkpointJSON, task)
	if err != nil {
		return BackgroundTask{}, err
	}
	if err := tx.Commit(); err != nil {
		return BackgroundTask{}, fmt.Errorf("commit background operation attachment: %w", err)
	}
	return attachedTask, nil
}

func (s *Store) attachBackgroundTaskAndRevealOperation(q Querier, operationID string, checkpointJSON []byte, task NewBackgroundTask) (BackgroundTask, error) {
	if q == nil {
		return BackgroundTask{}, errors.New("background operation querier is required")
	}
	if operationID == "" {
		return BackgroundTask{}, errors.New("background operation id is required")
	}

	// Make the first statement a write for standalone attachment transactions.
	// With SQLite WAL, reading first and then upgrading the snapshot to a writer
	// can fail immediately with SQLITE_BUSY if another writer commits between
	// the read and write. Atomic producer transactions may already own the writer
	// slot; repeating this checkpoint write is still part of the same commit.
	if err := s.setBackgroundOperationCheckpoint(q, operationID, checkpointJSON); err != nil {
		return BackgroundTask{}, err
	}

	var visible int
	var status string
	var attached int
	if err := q.QueryRow(`
		SELECT visible, status,
		       (SELECT count(*) FROM background_tasks WHERE operation_id = background_operations.id)
		FROM background_operations
		WHERE id = ?
	`, operationID).Scan(&visible, &status, &attached); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return BackgroundTask{}, errors.New("background operation is missing")
		}
		return BackgroundTask{}, fmt.Errorf("inspect background operation attachment: %w", err)
	}
	if (visible != 0 && visible != 1) || status != string(BackgroundWorkPending) || attached != 0 {
		return BackgroundTask{}, errors.New("background operation is not an unattached pending reservation")
	}

	task.OperationID = operationID
	attachedTask, created, err := s.EnqueueBackgroundTask(q, task)
	if err != nil {
		return BackgroundTask{}, fmt.Errorf("attach background child task: %w", err)
	}
	if !created {
		return BackgroundTask{}, errors.New("attach background child task: scoped dedupe key already active")
	}
	if err := s.setBackgroundOperationVisible(q, operationID, true); err != nil {
		return BackgroundTask{}, fmt.Errorf("reveal background operation: %w", err)
	}
	return attachedTask, nil
}
