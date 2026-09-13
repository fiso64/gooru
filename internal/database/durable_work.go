package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// BackgroundWorkStatus is the durable lifecycle shared by operations and tasks.
type BackgroundWorkStatus string

const (
	BackgroundWorkPending   BackgroundWorkStatus = "pending"
	BackgroundWorkRunning   BackgroundWorkStatus = "running"
	BackgroundWorkCompleted BackgroundWorkStatus = "completed"
	BackgroundWorkFailed    BackgroundWorkStatus = "failed"
	BackgroundWorkCanceled  BackgroundWorkStatus = "canceled"
)

const backgroundTaskEnqueueRaceRetries = 8

// BackgroundOperation is a user-visible (or deliberately hidden) logical unit of work.
type BackgroundOperation struct {
	ID                string
	Kind              string
	Visible           bool
	Status            BackgroundWorkStatus
	ProgressTotal     int64
	ProgressCompleted int64
	ProgressFailed    int64
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	ErrorCode         string
	ErrorMessage      string
}

// BackgroundTask is one durable schedulable unit beneath an operation, or independent
// background work when OperationID is empty.
type BackgroundTask struct {
	ID               string
	OperationID      string
	DedupeKey        string
	Kind             string
	SubjectKind      string
	SubjectID        string
	InputKey         string
	ResourceClass    string
	Priority         int
	Status           BackgroundWorkStatus
	AvailableAt      time.Time
	LeaseOwner       string
	LeaseExpiresAt   *time.Time
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	AttemptCount     int
	MaxAttempts      int
	LastErrorCode    string
	LastErrorMessage string
}

// NewBackgroundOperation describes a new logical operation. CreatedAt defaults to now
// when omitted. ProgressTotal may be zero when the total is not known at creation time.
type NewBackgroundOperation struct {
	ID            string
	Kind          string
	Visible       bool
	ProgressTotal int64
	CreatedAt     time.Time
}

// BackgroundTaskCleanup describes detached compensation to enqueue atomically if a
// task exhausts its retry budget. It deliberately cannot own another cleanup, so a
// permanently failing compensation task cannot recursively create more work.
type BackgroundTaskCleanup struct {
	DedupeKey     string `json:"dedupe_key"`
	Kind          string `json:"kind"`
	SubjectKind   string `json:"subject_kind,omitempty"`
	SubjectID     string `json:"subject_id,omitempty"`
	InputKey      string `json:"input_key,omitempty"`
	ResourceClass string `json:"resource_class,omitempty"`
	Priority      int    `json:"priority,omitempty"`
	MaxAttempts   int    `json:"max_attempts,omitempty"`
}

// NewBackgroundTask describes a pending durable task. AvailableAt and CreatedAt default
// to now when omitted. ResourceClass defaults to "default" and MaxAttempts defaults to 3.
type NewBackgroundTask struct {
	ID                     string
	OperationID            string
	DedupeKey              string
	Kind                   string
	SubjectKind            string
	SubjectID              string
	InputKey               string
	ResourceClass          string
	Priority               int
	AvailableAt            time.Time
	CreatedAt              time.Time
	MaxAttempts            int
	TerminalFailureCleanup *BackgroundTaskCleanup
}

// CreateBackgroundOperation inserts a logical operation. It accepts a Querier so callers
// can compose operation creation and task enqueueing in one transaction.
func (s *Store) CreateBackgroundOperation(q Querier, op NewBackgroundOperation) (BackgroundOperation, error) {
	if q == nil {
		return BackgroundOperation{}, errors.New("background operation querier is required")
	}
	if op.ID == "" {
		return BackgroundOperation{}, errors.New("background operation id is required")
	}
	if op.Kind == "" {
		return BackgroundOperation{}, errors.New("background operation kind is required")
	}
	if op.ProgressTotal < 0 {
		return BackgroundOperation{}, errors.New("background operation progress total cannot be negative")
	}
	createdAt := normalizeWorkTime(op.CreatedAt)
	visible := 0
	if op.Visible {
		visible = 1
	}
	_, err := q.Exec(`
		INSERT INTO background_operations
			(id, kind, visible, status, progress_total, created_at)
		VALUES (?, ?, ?, 'pending', ?, ?)
	`, op.ID, op.Kind, visible, op.ProgressTotal, workTimeValue(createdAt))
	if err != nil {
		return BackgroundOperation{}, fmt.Errorf("create background operation: %w", err)
	}
	return BackgroundOperation{
		ID:            op.ID,
		Kind:          op.Kind,
		Visible:       op.Visible,
		Status:        BackgroundWorkPending,
		ProgressTotal: op.ProgressTotal,
		CreatedAt:     createdAt,
	}, nil
}

// EnqueueBackgroundTask inserts a pending task unless the same dedupe key already has
// active work. In that case it returns the existing active task and created=false. The
// database partial unique index remains the concurrency authority; this method does not
// use a racy read-before-insert check. If the conflicting task becomes terminal before
// it can be read back, enqueueing retries so the requested work is not spuriously lost.
func (s *Store) EnqueueBackgroundTask(q Querier, task NewBackgroundTask) (result BackgroundTask, created bool, err error) {
	if q == nil {
		return BackgroundTask{}, false, errors.New("background task querier is required")
	}
	if task.ID == "" {
		return BackgroundTask{}, false, errors.New("background task id is required")
	}
	if task.DedupeKey == "" {
		return BackgroundTask{}, false, errors.New("background task dedupe key is required")
	}
	if task.Kind == "" {
		return BackgroundTask{}, false, errors.New("background task kind is required")
	}
	if task.ResourceClass == "" {
		task.ResourceClass = "default"
	}
	if task.MaxAttempts == 0 {
		task.MaxAttempts = 3
	}
	if task.MaxAttempts < 1 {
		return BackgroundTask{}, false, errors.New("background task max attempts must be positive")
	}
	terminalCleanupJSON, err := encodeBackgroundTaskCleanup(task.TerminalFailureCleanup)
	if err != nil {
		return BackgroundTask{}, false, err
	}
	task.CreatedAt = normalizeWorkTime(task.CreatedAt)
	if task.AvailableAt.IsZero() {
		task.AvailableAt = task.CreatedAt
	} else {
		task.AvailableAt = task.AvailableAt.UTC()
	}

	var operationID interface{}
	if task.OperationID != "" {
		operationID = task.OperationID
	}
	for attempt := 0; attempt < backgroundTaskEnqueueRaceRetries; attempt++ {
		res, err := q.Exec(`
			INSERT INTO background_tasks
				(id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
				 resource_class, priority, status, available_at, created_at, max_attempts,
				 terminal_cleanup_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?)
			ON CONFLICT(dedupe_key) WHERE status IN ('pending', 'running') DO NOTHING
		`, task.ID, operationID, task.DedupeKey, task.Kind, task.SubjectKind, task.SubjectID, task.InputKey,
			task.ResourceClass, task.Priority, workTimeValue(task.AvailableAt), workTimeValue(task.CreatedAt), task.MaxAttempts,
			terminalCleanupJSON)
		if err != nil {
			return BackgroundTask{}, false, fmt.Errorf("enqueue background task: %w", err)
		}
		rows, err := res.RowsAffected()
		if err != nil {
			return BackgroundTask{}, false, fmt.Errorf("enqueue background task rows affected: %w", err)
		}
		if rows == 1 {
			return BackgroundTask{
				ID:            task.ID,
				OperationID:   task.OperationID,
				DedupeKey:     task.DedupeKey,
				Kind:          task.Kind,
				SubjectKind:   task.SubjectKind,
				SubjectID:     task.SubjectID,
				InputKey:      task.InputKey,
				ResourceClass: task.ResourceClass,
				Priority:      task.Priority,
				Status:        BackgroundWorkPending,
				AvailableAt:   task.AvailableAt,
				CreatedAt:     task.CreatedAt,
				MaxAttempts:   task.MaxAttempts,
			}, true, nil
		}

		existing, err := scanBackgroundTask(q.QueryRow(`
			SELECT id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
			       resource_class, priority, status, available_at, lease_owner, lease_expires_at,
			       created_at, started_at, finished_at, attempt_count, max_attempts,
			       last_error_code, last_error_message
			FROM background_tasks
			WHERE dedupe_key = ? AND status IN ('pending', 'running')
			LIMIT 1
		`, task.DedupeKey))
		if err == nil {
			return existing, false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return BackgroundTask{}, false, fmt.Errorf("read active background task after dedupe conflict: %w", err)
		}
	}
	return BackgroundTask{}, false, errors.New("enqueue background task: active dedupe state kept changing")
}

func encodeBackgroundTaskCleanup(cleanup *BackgroundTaskCleanup) (string, error) {
	if cleanup == nil {
		return "", nil
	}
	normalized := *cleanup
	if normalized.DedupeKey == "" {
		return "", errors.New("background terminal cleanup dedupe key is required")
	}
	if normalized.Kind == "" {
		return "", errors.New("background terminal cleanup kind is required")
	}
	if normalized.ResourceClass == "" {
		normalized.ResourceClass = "default"
	}
	if normalized.MaxAttempts == 0 {
		normalized.MaxAttempts = 3
	}
	if normalized.MaxAttempts < 1 {
		return "", errors.New("background terminal cleanup max attempts must be positive")
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode background terminal cleanup: %w", err)
	}
	return string(encoded), nil
}

// GetBackgroundTaskForOperation returns the earliest durable child task for one
// logical operation. Callers that need arbitrary task history should use a separate
// bounded list API rather than treating this representative child as exhaustive.
func (s *Store) GetBackgroundTaskForOperation(operationID string) (BackgroundTask, bool, error) {
	if operationID == "" {
		return BackgroundTask{}, false, errors.New("background operation id is required")
	}
	task, err := scanBackgroundTask(s.DB.QueryRow(`
		SELECT id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key,
		       resource_class, priority, status, available_at, lease_owner, lease_expires_at,
		       created_at, started_at, finished_at, attempt_count, max_attempts,
		       last_error_code, last_error_message
		FROM background_tasks
		WHERE operation_id = ?
		ORDER BY created_at ASC, id ASC
		LIMIT 1
	`, operationID))
	if errors.Is(err, sql.ErrNoRows) {
		return BackgroundTask{}, false, nil
	}
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("get background task for operation: %w", err)
	}
	return task, true, nil
}

func scanBackgroundTask(row *sql.Row) (BackgroundTask, error) {
	var task BackgroundTask
	var operationID sql.NullString
	var status string
	var availableAt int64
	var leaseExpiresAt sql.NullInt64
	var createdAt int64
	var startedAt sql.NullInt64
	var finishedAt sql.NullInt64
	if err := row.Scan(
		&task.ID, &operationID, &task.DedupeKey, &task.Kind, &task.SubjectKind, &task.SubjectID,
		&task.InputKey, &task.ResourceClass, &task.Priority, &status, &availableAt, &task.LeaseOwner,
		&leaseExpiresAt, &createdAt, &startedAt, &finishedAt, &task.AttemptCount, &task.MaxAttempts,
		&task.LastErrorCode, &task.LastErrorMessage,
	); err != nil {
		return BackgroundTask{}, err
	}
	task.OperationID = operationID.String
	task.Status = BackgroundWorkStatus(status)
	task.AvailableAt = workTime(availableAt)
	task.CreatedAt = workTime(createdAt)
	task.LeaseExpiresAt = nullableWorkTime(leaseExpiresAt)
	task.StartedAt = nullableWorkTime(startedAt)
	task.FinishedAt = nullableWorkTime(finishedAt)
	return task, nil
}

func normalizeWorkTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func workTimeValue(value time.Time) int64 {
	return value.UTC().UnixMilli()
}

func workTime(value int64) time.Time {
	return time.UnixMilli(value).UTC()
}

func nullableWorkTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := workTime(value.Int64)
	return &t
}
