package gooru

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

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
	id, err := newBackgroundTaskID()
	if err != nil {
		return BackgroundTask{}, false, err
	}
	return enqueueDatabaseBackgroundTask(c, q, id, request)
}

func newBackgroundTaskID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate background task id: %w", err)
	}
	return "task-" + hex.EncodeToString(random[:]), nil
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
