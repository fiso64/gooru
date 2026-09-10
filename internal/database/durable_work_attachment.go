package database

import (
	"database/sql"
	"errors"
	"fmt"
)

// AttachBackgroundTaskAndRevealOperation atomically persists producer recovery
// state, attaches the first durable child task to a hidden admitted operation,
// and reveals that operation to consumers. A failed attachment rolls all three
// mutations back so startup recovery can safely release the still-hidden
// reservation.
func (s *Store) AttachBackgroundTaskAndRevealOperation(operationID string, checkpointJSON []byte, task NewBackgroundTask) (BackgroundTask, error) {
	if s == nil || s.DB == nil {
		return BackgroundTask{}, errors.New("background operation store is required")
	}
	if operationID == "" {
		return BackgroundTask{}, errors.New("background operation id is required")
	}

	tx, err := s.Begin()
	if err != nil {
		return BackgroundTask{}, fmt.Errorf("begin background operation attachment: %w", err)
	}
	defer tx.Rollback()

	// Make the first statement a write. With SQLite WAL, reading first and then
	// upgrading the snapshot to a writer can fail immediately with SQLITE_BUSY if
	// an idle durable worker commits between the read and write; busy_timeout does
	// not make that stale snapshot upgrade retryable. The checkpoint write acquires
	// writer ownership up front, and any later reservation-validation failure rolls
	// it back with the rest of this transaction.
	if err := s.setBackgroundOperationCheckpoint(tx, operationID, checkpointJSON); err != nil {
		return BackgroundTask{}, err
	}

	var visible int
	var status string
	var attached int
	if err := tx.QueryRow(`
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
	if visible != 0 || status != string(BackgroundWorkPending) || attached != 0 {
		return BackgroundTask{}, errors.New("background operation is not an unattached hidden pending reservation")
	}

	task.OperationID = operationID
	attachedTask, created, err := s.EnqueueBackgroundTask(tx, task)
	if err != nil {
		return BackgroundTask{}, fmt.Errorf("attach background child task: %w", err)
	}
	if !created {
		return BackgroundTask{}, errors.New("attach background child task: scoped dedupe key already active")
	}
	if err := s.setBackgroundOperationVisible(tx, operationID, true); err != nil {
		return BackgroundTask{}, fmt.Errorf("reveal background operation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BackgroundTask{}, fmt.Errorf("commit background operation attachment: %w", err)
	}
	return attachedTask, nil
}
