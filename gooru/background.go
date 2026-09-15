package gooru

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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
	operation, err := c.createBackgroundOperation(c.store.DB, request)
	if err == nil && operation.Visible {
		c.notifyBackgroundOperationChange()
	}
	return operation, err
}

// CreateBackgroundOperationWithTasks atomically creates one logical operation
// and its initial child tasks. Child dedupe keys are scoped to the new operation
// so a separate user request cannot accidentally attach its progress to active
// work owned by another operation. An empty child dedupe key is allowed here and
// receives a stable per-operation key derived from its position in the batch.
func (c *Client) CreateBackgroundOperationWithTasks(operationRequest BackgroundOperationRequest, taskRequests []BackgroundTaskRequest) (BackgroundOperation, []BackgroundTask, error) {
	operation, tasks, _, err := c.createBackgroundOperationWithTasks(operationRequest, taskRequests, false)
	return operation, tasks, err
}

// createBackgroundOperationWithTasksSingleFlight creates one operation and its
// child tasks only when no pending/running operation of the same kind exists.
// The active lookup and creation share the Store's immediate transaction, so
// independent clients cannot both pass the lookup and create duplicate work.
func (c *Client) createBackgroundOperationWithTasksSingleFlight(operationRequest BackgroundOperationRequest, taskRequests []BackgroundTaskRequest) (BackgroundOperation, []BackgroundTask, bool, error) {
	return c.createBackgroundOperationWithTasks(operationRequest, taskRequests, true)
}

func (c *Client) createBackgroundOperationWithTasks(operationRequest BackgroundOperationRequest, taskRequests []BackgroundTaskRequest, singleFlight bool) (BackgroundOperation, []BackgroundTask, bool, error) {
	tx, err := c.store.Begin()
	if err != nil {
		return BackgroundOperation{}, nil, false, fmt.Errorf("begin background operation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if singleFlight {
		_, found, err := c.store.FindActiveBackgroundOperationIDByKind(tx, operationRequest.Kind)
		if err != nil {
			return BackgroundOperation{}, nil, false, fmt.Errorf("inspect active background operation: %w", err)
		}
		if found {
			return BackgroundOperation{}, nil, false, nil
		}
	}

	operation, err := c.createBackgroundOperation(tx, operationRequest)
	if err != nil {
		return BackgroundOperation{}, nil, false, err
	}

	tasks := make([]BackgroundTask, 0, len(taskRequests))
	for index, request := range taskRequests {
		if request.OperationID != "" {
			return BackgroundOperation{}, nil, false, fmt.Errorf("background child task %d already belongs to operation %q", index, request.OperationID)
		}
		if request.Operation != nil {
			return BackgroundOperation{}, nil, false, fmt.Errorf("background child task %d requests a nested operation", index)
		}
		request.OperationID = operation.ID
		if request.DedupeKey == "" {
			request.DedupeKey = fmt.Sprintf("task:%d", index)
		}
		request.DedupeKey = operation.ID + ":" + request.DedupeKey
		task, created, err := c.enqueueBackgroundTask(tx, request)
		if err != nil {
			return BackgroundOperation{}, nil, false, fmt.Errorf("enqueue background child task %d: %w", index, err)
		}
		if !created {
			return BackgroundOperation{}, nil, false, fmt.Errorf("enqueue background child task %d: scoped dedupe key already active", index)
		}
		tasks = append(tasks, task)
	}

	if err := tx.Commit(); err != nil {
		return BackgroundOperation{}, nil, false, fmt.Errorf("commit background operation transaction: %w", err)
	}
	if operation.Visible {
		c.notifyBackgroundOperationChange()
	}
	return operation, tasks, true, nil
}

// ClaimBackgroundOperationProducer atomically grants one producer the right to
// stage and attach the first child task for an admitted operation.
func (c *Client) ClaimBackgroundOperationProducer(operationID string) (bool, error) {
	if operationID == "" {
		return false, fmt.Errorf("background operation id is required")
	}
	return c.store.ClaimBackgroundOperationProducer(operationID)
}

// AttachBackgroundTaskAndRevealOperation finishes producer admission after
// expensive staging. The recovery checkpoint, first child task, and final
// visibility state commit atomically. Producers normally attach hidden work;
// uploads may already be visible while their request body is still arriving.
func (c *Client) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request BackgroundTaskRequest) (BackgroundTask, error) {
	if operationID == "" {
		return BackgroundTask{}, fmt.Errorf("background operation id is required")
	}
	if request.Operation != nil {
		return BackgroundTask{}, fmt.Errorf("background child task requests a nested operation")
	}
	if request.OperationID != "" && request.OperationID != operationID {
		return BackgroundTask{}, fmt.Errorf("background child task already belongs to operation %q", request.OperationID)
	}
	checkpointJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return BackgroundTask{}, fmt.Errorf("encode background operation checkpoint: %w", err)
	}
	if request.DedupeKey == "" {
		request.DedupeKey = "task:0"
	}
	request.OperationID = operationID
	request.DedupeKey = operationID + ":" + request.DedupeKey
	taskID, err := newBackgroundWorkID("task")
	if err != nil {
		return BackgroundTask{}, err
	}
	task, err := attachDatabaseBackgroundTaskAndRevealOperation(c, operationID, checkpointJSON, taskID, request)
	if err == nil {
		c.notifyBackgroundOperationChange()
	}
	return task, err
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
	canceled, err := cancelDatabaseBackgroundOperation(c, operationID)
	if err == nil && canceled {
		c.notifyBackgroundOperationChange()
	}
	return canceled, err
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

// BackgroundTaskCleanupRequest describes detached compensation to enqueue
// atomically if the owning task exhausts its retry budget. Compensation cannot
// recursively own another compensation task.
type BackgroundTaskCleanupRequest struct {
	DedupeKey     string
	Kind          string
	SubjectKind   string
	SubjectID     string
	InputKey      string
	ResourceClass string
	Priority      int
	MaxAttempts   int
}

// BackgroundTaskRequest describes durable work to enqueue. DedupeKey is the
// stable identity of active equivalent work; callers should include every input
// that changes the promised result. AvailableAt is optional and defaults to now.
// Operation requests an owning logical operation that is created atomically with
// the task when the producer is already inside a domain transaction. It is
// mutually exclusive with OperationID.
type BackgroundTaskRequest struct {
	OperationID            string
	Operation              *BackgroundOperationRequest
	DedupeKey              string
	Kind                   string
	SubjectKind            string
	SubjectID              string
	InputKey               string
	ResourceClass          string
	Priority               int
	AvailableAt            time.Time
	MaxAttempts            int
	TerminalFailureCleanup *BackgroundTaskCleanupRequest
	reuseActiveOperation   bool
}

// EnqueueBackgroundTask persists durable work outside an existing transaction.
// It returns the active equivalent task with created=false when DedupeKey is
// already pending or running. Requests that declare Operation are routed through
// the operation+task transaction so an enqueue failure cannot orphan the parent.
func (c *Client) EnqueueBackgroundTask(request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	if request.Operation != nil {
		if request.OperationID != "" {
			return BackgroundTask{}, false, fmt.Errorf("background task cannot declare both operation id and operation request")
		}
		operationRequest := *request.Operation
		request.Operation = nil
		_, tasks, err := c.CreateBackgroundOperationWithTasks(operationRequest, []BackgroundTaskRequest{request})
		if err != nil {
			return BackgroundTask{}, false, err
		}
		if len(tasks) != 1 {
			return BackgroundTask{}, false, fmt.Errorf("background operation enqueue created %d tasks, want 1", len(tasks))
		}
		return tasks[0], true, nil
	}

	task, created, err = c.enqueueBackgroundTask(c.store.DB, request)
	if err == nil && created && task.OperationID != "" {
		c.notifyBackgroundOperationChange()
	}
	return task, created, err
}

// enqueueBackgroundTask is the transaction-aware core primitive used by
// business mutations that need content registration and background work to
// commit atomically.
func (c *Client) enqueueBackgroundTask(q databaseQuerier, request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	if request.Operation != nil {
		if request.OperationID != "" {
			return BackgroundTask{}, false, fmt.Errorf("background task cannot declare both operation id and operation request")
		}
		operationRequest := *request.Operation
		request.Operation = nil

		operationID := ""
		if request.reuseActiveOperation {
			activeID, found, err := c.store.FindActiveBackgroundOperationIDByKind(q, operationRequest.Kind)
			if err != nil {
				return BackgroundTask{}, false, fmt.Errorf("find reusable background task operation: %w", err)
			}
			if found {
				operationID = activeID
			}
		}
		if operationID == "" {
			operation, err := c.createBackgroundOperation(q, operationRequest)
			if err != nil {
				return BackgroundTask{}, false, fmt.Errorf("create background task operation: %w", err)
			}
			operationID = operation.ID
		}
		request.OperationID = operationID
		if request.DedupeKey == "" {
			request.DedupeKey = "task:0"
		}
		request.DedupeKey = operationID + ":" + request.DedupeKey
	}

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
	canceled, err := cancelDatabaseBackgroundTask(c, taskID)
	if err == nil && canceled {
		c.notifyBackgroundOperationChange()
	}
	return canceled, err
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
