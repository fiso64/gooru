package gooru

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// BackgroundOperation is the core-facing representation of one logical durable
// operation. It intentionally exposes only stable operation identity and the
// creation-time progress contract; mutable runtime bookkeeping remains behind
// the persistence/runtime boundary until consumers need an explicit read API.
type BackgroundOperation struct {
	ID            string
	Kind          string
	Visible       bool
	ProgressTotal int64
	CreatedAt     time.Time
}

// BackgroundOperationRequest describes one logical operation to create.
// ProgressTotal may be zero when the total is not known yet.
type BackgroundOperationRequest struct {
	Kind          string
	Visible       bool
	ProgressTotal int64
}

// CreateBackgroundOperation persists one logical operation outside an existing
// transaction. Producers that must atomically create an operation with domain
// state and tasks should use createBackgroundOperation from the core mutation.
func (c *Client) CreateBackgroundOperation(request BackgroundOperationRequest) (BackgroundOperation, error) {
	return c.createBackgroundOperation(c.store.DB, request)
}

func (c *Client) createBackgroundOperation(q databaseQuerier, request BackgroundOperationRequest) (BackgroundOperation, error) {
	id, err := newBackgroundWorkID("operation")
	if err != nil {
		return BackgroundOperation{}, err
	}
	return createDatabaseBackgroundOperation(c, q, id, request)
}

// CancelBackgroundOperation durably cancels an active logical operation and all
// pending/running child work. A running worker loses its lease and receives
// context cancellation through BackgroundRuntime's renewal boundary.
func (c *Client) CancelBackgroundOperation(operationID string) (bool, error) {
	return cancelDatabaseBackgroundOperation(c, operationID)
}

// BackgroundTask is the core-facing input for one claimed durable task. It
// intentionally exposes only stable domain identity/input fields: lease state,
// retry bookkeeping, scheduling metadata, and terminal status remain runtime
// implementation details.
type BackgroundTask struct {
	ID          string
	OperationID string
	Kind        string
	SubjectKind string
	SubjectID   string
	InputKey    string
}

// BackgroundTaskRequest describes durable work to enqueue. DedupeKey is the
// stable identity of active equivalent work; callers should include every input
// that changes the promised result. AvailableAt is optional and defaults to now.
type BackgroundTaskRequest struct {
	OperationID   string
	DedupeKey     string
	Kind          string
	SubjectKind   string
	SubjectID     string
	InputKey      string
	ResourceClass string
	Priority      int
	AvailableAt   time.Time
	MaxAttempts   int
}

// EnqueueBackgroundTask persists durable work outside an existing transaction.
// It returns the active equivalent task with created=false when DedupeKey is
// already pending or running.
func (c *Client) EnqueueBackgroundTask(request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	return c.enqueueBackgroundTask(c.store.DB, request)
}

// enqueueBackgroundTask is the transaction-aware core primitive used by
// business mutations that need content registration and background work to
// commit atomically.
func (c *Client) enqueueBackgroundTask(q databaseQuerier, request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	id, err := newBackgroundWorkID("task")
	if err != nil {
		return BackgroundTask{}, false, err
	}
	return enqueueDatabaseBackgroundTask(c, q, id, request)
}

// CancelBackgroundTask durably cancels one pending/running task. This is the
// lower-level cancellation primitive for consumers that expose per-item cancel;
// canceling the parent operation is preferred for whole-operation cancellation.
func (c *Client) CancelBackgroundTask(taskID string) (bool, error) {
	return cancelDatabaseBackgroundTask(c, taskID)
}

func newBackgroundWorkID(prefix string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate background %s id: %w", prefix, err)
	}
	return prefix + "-" + hex.EncodeToString(random[:]), nil
}

// BackgroundTaskHandler executes one claimed durable task. The task lease and
// completion/retry lifecycle are owned by BackgroundRuntime; handlers only own
// the domain operation itself.
type BackgroundTaskHandler func(context.Context, BackgroundTask) error

// BackgroundWorkerConfig describes one durable worker class. Handlers are keyed
// by persisted task kind. Timing fields are optional; zero values use runtime
// defaults.
type BackgroundWorkerConfig struct {
	ResourceClass string
	WorkerID      string
	Handlers      map[string]BackgroundTaskHandler
	LeaseDuration time.Duration
	PollInterval  time.Duration
	RetryDelay    time.Duration
}

// BackgroundRuntime owns restart-safe claiming, lease renewal and terminal task
// transitions for one durable worker class.
type BackgroundRuntime interface {
	Run(context.Context) error
}

// NewBackgroundRuntime composes the durable task runner for this client without
// exposing persistence implementation details to callers or task handlers.
func (c *Client) NewBackgroundRuntime(cfg BackgroundWorkerConfig) (BackgroundRuntime, error) {
	return newDatabaseBackgroundRuntime(c, cfg)
}
