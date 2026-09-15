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

type backgroundOperationCreationMode uint8

const (
	backgroundOperationCreateAlways backgroundOperationCreationMode = iota
	backgroundOperationCreateSingleFlight
)

// CreateBackgroundOperationWithTasks atomically creates one logical operation
// and its initial child tasks. Child dedupe keys are scoped to the new operation
// so a separate user request cannot accidentally attach its progress to active
// work owned by another operation. An empty child dedupe key is allowed here and
// receives a stable per-operation key derived from its position in the batch.
func (c *Client) CreateBackgroundOperationWithTasks(operationRequest BackgroundOperationRequest, taskRequests []BackgroundTaskRequest) (BackgroundOperation, []BackgroundTask, error) {
	operation, tasks, _, err := c.createBackgroundOperationWithTasks(operationRequest, taskRequests, backgroundOperationCreateAlways)
	return operation, tasks, err
}

// createBackgroundOperationWithTasksSingleFlight creates one operation and its
// child tasks only when no pending/running operation of the same kind exists.
// The active lookup and creation share the Store's immediate transaction, so
// independent clients cannot both pass the lookup and create duplicate work.
func (c *Client) createBackgroundOperationWithTasksSingleFlight(operationRequest BackgroundOperationRequest, taskRequests []BackgroundTaskRequest) (BackgroundOperation, []BackgroundTask, bool, error) {
	return c.createBackgroundOperationWithTasks(operationRequest, taskRequests, backgroundOperationCreateSingleFlight)
}

func (c *Client) createBackgroundOperationWithTasks(operationRequest BackgroundOperationRequest, taskRequests []BackgroundTaskRequest, mode backgroundOperationCreationMode) (BackgroundOperation, []BackgroundTask, bool, error) {
	tx, err := c.store.Begin()
	if err != nil {
		return BackgroundOperation{}, nil, false, fmt.Errorf("begin background operation transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	switch mode {
	case backgroundOperationCreateAlways:
	case backgroundOperationCreateSingleFlight:
		_, found, err := c.store.FindActiveBackgroundOperationIDByKind(tx, operationRequest.Kind)
		if err != nil {
			return BackgroundOperation{}, nil, false, fmt.Errorf("inspect active background operation: %w", err)
		}
		if found {
			return BackgroundOperation{}, nil, false, nil
		}
	default:
		return BackgroundOperation{}, nil, false, fmt.Errorf("unsupported background operation creation mode %d", mode)
	}

	operation, err := c.createBackgroundOperation(tx, operationRequest)
	if err != nil {
		return BackgroundOperation{}, nil, false, err
	}

	tasks := make([]BackgroundTask, 0, len(taskRequests))
	for index, request := range taskRequests {
		request, err = bindBackgroundChildTask(request, operation.ID, index)
		if err != nil {
			return BackgroundOperation{}, nil, false, fmt.Errorf("bind background child task %d: %w", index, err)
		}
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
	var err error
	request, err = bindBackgroundChildTask(request, operationID, 0)
	if err != nil {
		return BackgroundTask{}, err
	}
	checkpointJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return BackgroundTask{}, fmt.Errorf("encode background operation checkpoint: %w", err)
	}
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

func bindBackgroundChildTask(request BackgroundTaskRequest, operationID string, index int) (BackgroundTaskRequest, error) {
	if operationID == "" {
		return BackgroundTaskRequest{}, fmt.Errorf("background operation id is required")
	}
	if request.Operation != nil {
		return BackgroundTaskRequest{}, fmt.Errorf("background child task requests a nested operation")
	}
	if request.OperationBinding != BackgroundOperationCreateNew {
		return BackgroundTaskRequest{}, fmt.Errorf("background child task requests an operation binding policy")
	}
	if request.OperationID != "" && request.OperationID != operationID {
		return BackgroundTaskRequest{}, fmt.Errorf("background child task already belongs to operation %q", request.OperationID)
	}
	request.OperationID = operationID
	if request.DedupeKey == "" {
		request.DedupeKey = fmt.Sprintf("task:%d", index)
	}
	request.DedupeKey = operationID + ":" + request.DedupeKey
	return request, nil
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

// BackgroundOperationBinding controls how an operation request attached to a
// task is resolved. The zero value creates a new owning operation. ReuseActive
// instead attaches to an active operation of the same kind when one exists,
// creating that operation only when necessary. The lookup/create decision is
// made in the caller's transaction so it is safe across independent clients.
type BackgroundOperationBinding uint8

const (
	BackgroundOperationCreateNew BackgroundOperationBinding = iota
	BackgroundOperationReuseActive
)

// BackgroundTaskRequest describes durable work to enqueue. DedupeKey is the
// stable identity of active equivalent work; callers should include every input
// that changes the promised result. AvailableAt is optional and defaults to now.
// Operation requests an owning logical operation that is created atomically with
// the task when the producer is already inside a domain transaction. It is
// mutually exclusive with OperationID. OperationBinding explicitly controls
// whether that operation must be new or may reuse active work of the same kind.
// CoalescePendingEquivalent is a narrower opt-in for reusable operations: after
// resolving the operation, an equivalent pending task may be reused, while a
// running task never suppresses a newly committed wake. PostponePendingEquivalent
// additionally moves a reused pending task's availability later to AvailableAt,
// enabling a trailing-edge debounce without allowing running work to absorb a
// newly committed wake.
type BackgroundTaskRequest struct {
	OperationID                string
	Operation                  *BackgroundOperationRequest
	OperationBinding           BackgroundOperationBinding
	CoalescePendingEquivalent  bool
	PostponePendingEquivalent  bool
	DedupeKey                  string
	Kind                       string
	SubjectKind                string
	SubjectID                  string
	InputKey                   string
	ResourceClass              string
	Priority                   int
	AvailableAt                time.Time
	MaxAttempts                int
	TerminalFailureCleanup     *BackgroundTaskCleanupRequest
}

// EnqueueBackgroundTask persists durable work outside an existing transaction.
// It returns the active equivalent task with created=false when DedupeKey is
// already pending or running. Operation-backed requests use the same transaction-
// aware binding path as domain mutations, so create/reuse policy is interpreted
// consistently regardless of where the request originates.
func (c *Client) EnqueueBackgroundTask(request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	if request.Operation == nil {
		if request.OperationBinding != BackgroundOperationCreateNew {
			return BackgroundTask{}, false, fmt.Errorf("background operation binding requires an operation request")
		}
		task, created, err = c.enqueueBackgroundTask(c.store.DB, request)
		if err == nil && created && task.OperationID != "" {
			c.notifyBackgroundOperationChange()
		}
		return task, created, err
	}
	if request.OperationID != "" {
		return BackgroundTask{}, false, fmt.Errorf("background task cannot declare both operation id and operation request")
	}

	binding := request.OperationBinding
	tx, err := c.store.Begin()
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("begin background task transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	task, created, err = c.enqueueBackgroundTask(tx, request)
	if err != nil {
		return BackgroundTask{}, false, err
	}
	if !created && binding == BackgroundOperationCreateNew {
		return BackgroundTask{}, false, fmt.Errorf("new background operation child task dedupe key already active")
	}
	if err := tx.Commit(); err != nil {
		return BackgroundTask{}, false, fmt.Errorf("commit background task transaction: %w", err)
	}
	if created && task.OperationID != "" {
		c.notifyBackgroundOperationChange()
	}
	return task, created, nil
}

// enqueueBackgroundTask is the transaction-aware core primitive used by
// business mutations that need content registration and background work to
// commit atomically.
func (c *Client) enqueueBackgroundTask(q databaseQuerier, request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	if request.CoalescePendingEquivalent {
		if request.Operation == nil || request.OperationBinding != BackgroundOperationReuseActive {
			return BackgroundTask{}, false, fmt.Errorf("pending-equivalent background task coalescing requires a reusable operation request")
		}
		if request.InputKey == "" {
			return BackgroundTask{}, false, fmt.Errorf("pending-equivalent background task coalescing requires an input key")
		}
	}
	if request.PostponePendingEquivalent {
		if !request.CoalescePendingEquivalent {
			return BackgroundTask{}, false, fmt.Errorf("postponing a pending equivalent task requires pending-equivalent coalescing")
		}
		if request.AvailableAt.IsZero() {
			return BackgroundTask{}, false, fmt.Errorf("postponing a pending equivalent task requires an availability time")
		}
	}
	if request.Operation == nil {
		if request.OperationBinding != BackgroundOperationCreateNew {
			return BackgroundTask{}, false, fmt.Errorf("background operation binding requires an operation request")
		}
	} else {
		if request.OperationID != "" {
			return BackgroundTask{}, false, fmt.Errorf("background task cannot declare both operation id and operation request")
		}
		coalescePendingEquivalent := request.CoalescePendingEquivalent
		postponePendingEquivalent := request.PostponePendingEquivalent
		operationRequest := *request.Operation
		request.Operation = nil

		operationID := ""
		switch request.OperationBinding {
		case BackgroundOperationCreateNew:
		case BackgroundOperationReuseActive:
			activeID, found, err := c.store.FindActiveBackgroundOperationIDByKind(q, operationRequest.Kind)
			if err != nil {
				return BackgroundTask{}, false, fmt.Errorf("find reusable background task operation: %w", err)
			}
			if found {
				operationID = activeID
			}
		default:
			return BackgroundTask{}, false, fmt.Errorf("unsupported background operation binding %d", request.OperationBinding)
		}
		if operationID == "" {
			operation, err := c.createBackgroundOperation(q, operationRequest)
			if err != nil {
				return BackgroundTask{}, false, fmt.Errorf("create background task operation: %w", err)
			}
			operationID = operation.ID
		}
		request.OperationBinding = BackgroundOperationCreateNew
		request, err = bindBackgroundChildTask(request, operationID, 0)
		if err != nil {
			return BackgroundTask{}, false, err
		}
		if coalescePendingEquivalent {
			existing, found, err := c.store.FindPendingBackgroundTask(q, request.OperationID, request.Kind, request.SubjectKind, request.SubjectID, request.InputKey, request.ResourceClass)
			if err != nil {
				return BackgroundTask{}, false, fmt.Errorf("find pending equivalent background task: %w", err)
			}
			if found {
				if postponePendingEquivalent && request.AvailableAt.After(existing.AvailableAt) {
					postponed, err := c.store.PostponePendingBackgroundTask(q, existing.ID, request.AvailableAt)
					if err != nil {
						return BackgroundTask{}, false, fmt.Errorf("postpone pending equivalent background task: %w", err)
					}
					if postponed {
						existing.AvailableAt = request.AvailableAt.UTC()
					}
				}
				return backgroundTaskFromDatabase(existing), false, nil
			}
		}
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
