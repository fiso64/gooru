package gooru

import (
	"context"
	"time"
)

// BackgroundTask is the core-facing representation of durable background work.
// It deliberately contains no database implementation types so task handlers can
// remain part of the library/runtime composition layer.
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
	Status           string
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
