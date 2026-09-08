package gooru

import "context"

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
