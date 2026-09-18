package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	defaultBackgroundOperationListLimit = 100
	maxBackgroundOperationListLimit     = 1000
)

// GetBackgroundOperation returns one durable logical operation by ID. The bool
// distinguishes a missing operation from a read failure without leaking sql.ErrNoRows
// into core callers.
func (s *Store) GetBackgroundOperation(id string) (BackgroundOperation, bool, error) {
	if id == "" {
		return BackgroundOperation{}, false, errors.New("background operation id is required")
	}
	operation, err := scanBackgroundOperation(s.DB.QueryRow(`
		SELECT id, kind, visible, status, progress_total, progress_completed, progress_failed,
		       created_at, started_at, finished_at, error_code, error_message
		FROM background_operations
		WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundOperation{}, false, nil
	}
	if err != nil {
		return BackgroundOperation{}, false, fmt.Errorf("read background operation: %w", err)
	}
	return operation, true, nil
}

// GetBackgroundOperations returns the durable states for the requested IDs.
// Missing IDs are omitted. Callers that care about request order should rebuild
// it from the returned map.
func (s *Store) GetBackgroundOperations(ids []string) (map[string]BackgroundOperation, error) {
	operations := make(map[string]BackgroundOperation, len(ids))
	if len(ids) == 0 {
		return operations, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		if id == "" {
			return nil, errors.New("background operation id is required")
		}
		args[i] = id
	}
	rows, err := s.DB.Query(`
		SELECT id, kind, visible, status, progress_total, progress_completed, progress_failed,
		       created_at, started_at, finished_at, error_code, error_message
		FROM background_operations
		WHERE id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("read background operations: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		operation, err := scanBackgroundOperation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan background operation: %w", err)
		}
		operations[operation.ID] = operation
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate background operations: %w", err)
	}
	return operations, nil
}

// GetBackgroundTask returns one durable task by its stable task ID. The bool
// distinguishes a missing task from a read failure without exposing sql.ErrNoRows
// to callers that need to recognize idempotent transport retries.
func (s *Store) GetBackgroundTask(id string) (BackgroundTask, bool, error) {
	if id == "" {
		return BackgroundTask{}, false, errors.New("background task id is required")
	}
	task, err := scanBackgroundTask(s.DB.QueryRow(`
		SELECT id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
		       resource_class, priority, status, available_at, lease_owner, lease_expires_at,
		       created_at, started_at, finished_at, attempt_count, max_attempts,
		       last_error_code, last_error_message
		FROM background_tasks
		WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundTask{}, false, nil
	}
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("read background task: %w", err)
	}
	return task, true, nil
}

// ListBackgroundOperations returns the first page of newest-first durable
// operation state. Use ListBackgroundOperationsPage when a non-zero offset is
// required.
func (s *Store) ListBackgroundOperations(visibleOnly bool, limit int) ([]BackgroundOperation, error) {
	return s.ListBackgroundOperationsPage(visibleOnly, limit, 0)
}

// ListBackgroundOperationsPage returns newest-first durable operation state.
// When visibleOnly is true, hidden implementation/background operations are
// omitted. A non-positive limit uses a bounded default; very large limits are
// capped so a diagnostics/UI read cannot accidentally materialize unbounded
// history. Negative offsets are treated as zero.
func (s *Store) ListBackgroundOperationsPage(visibleOnly bool, limit, offset int) ([]BackgroundOperation, error) {
	if limit <= 0 {
		limit = defaultBackgroundOperationListLimit
	} else if limit > maxBackgroundOperationListLimit {
		limit = maxBackgroundOperationListLimit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT id, kind, visible, status, progress_total, progress_completed, progress_failed,
		       created_at, started_at, finished_at, error_code, error_message
		FROM background_operations
	`
	if visibleOnly {
		query += " WHERE visible = 1"
	}
	query += " ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?"

	rows, err := s.DB.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list background operations: %w", err)
	}
	defer rows.Close()

	operations := make([]BackgroundOperation, 0)
	for rows.Next() {
		operation, err := scanBackgroundOperation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan background operation: %w", err)
		}
		operations = append(operations, operation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate background operations: %w", err)
	}
	return operations, nil
}

type backgroundOperationScanner interface {
	Scan(dest ...any) error
}

func scanBackgroundOperation(scanner backgroundOperationScanner) (BackgroundOperation, error) {
	var operation BackgroundOperation
	var visible int
	var status string
	var createdAt int64
	var startedAt sql.NullInt64
	var finishedAt sql.NullInt64
	if err := scanner.Scan(
		&operation.ID,
		&operation.Kind,
		&visible,
		&status,
		&operation.ProgressTotal,
		&operation.ProgressCompleted,
		&operation.ProgressFailed,
		&createdAt,
		&startedAt,
		&finishedAt,
		&operation.ErrorCode,
		&operation.ErrorMessage,
	); err != nil {
		return BackgroundOperation{}, err
	}
	operation.Visible = visible != 0
	operation.Status = BackgroundWorkStatus(status)
	operation.CreatedAt = workTime(createdAt)
	operation.StartedAt = nullableWorkTime(startedAt)
	operation.FinishedAt = nullableWorkTime(finishedAt)
	return operation, nil
}
