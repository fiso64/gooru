package gooru

import (
	"context"
	"fmt"
	"time"

	"gooru.local/internal/background"
	"gooru.local/internal/database"
)

// databaseTx, databaseQuerier, contentTagPair, and tagFacetExclusion keep
// database implementation types behind the core package's composition boundary.
// Business-logic files should depend on these package-local names rather than
// importing internal/database directly.
type databaseTx = database.Tx
type databaseQuerier = database.Querier
type contentTagPair = database.ContentTagPair
type tagFacetExclusion = database.TagFacetExclusion

var errInvalidSavedSearchOrder = database.ErrInvalidSavedSearchOrder

func createDatabaseBackgroundOperation(client *Client, q databaseQuerier, id string, request BackgroundOperationRequest) (BackgroundOperation, error) {
	operation, err := client.store.CreateBackgroundOperation(q, database.NewBackgroundOperation{
		ID:            id,
		Kind:          request.Kind,
		Visible:       request.Visible,
		ProgressTotal: request.ProgressTotal,
	})
	if err != nil {
		return BackgroundOperation{}, err
	}
	return backgroundOperationFromDatabase(operation), nil
}

func createDatabaseBackgroundTagMutationWithTask(
	client *Client,
	operationID string,
	taskID string,
	operationRequest BackgroundOperationRequest,
	maxPending int,
	mutation string,
	selectorJSON []byte,
	tagsJSON []byte,
	targetKind string,
	targetIDs []string,
	targetQuery string,
	targetArgs []interface{},
	checkpointJSON []byte,
	taskRequest BackgroundTaskRequest,
) (BackgroundOperation, bool, error) {
	operation, created, err := client.store.CreateBackgroundTagMutationWithTask(
		database.NewBackgroundOperation{
			ID:            operationID,
			Kind:          operationRequest.Kind,
			Visible:       operationRequest.Visible,
			ProgressTotal: operationRequest.ProgressTotal,
		},
		maxPending,
		mutation,
		selectorJSON,
		tagsJSON,
		targetKind,
		targetIDs,
		targetQuery,
		targetArgs,
		checkpointJSON,
		databaseBackgroundTask(taskID, taskRequest),
	)
	if err != nil || !created {
		return BackgroundOperation{}, created, err
	}
	return backgroundOperationFromDatabase(operation), true, nil
}

func getDatabaseBackgroundOperation(client *Client, operationID string) (BackgroundOperationState, bool, error) {
	operation, found, err := client.store.GetBackgroundOperation(operationID)
	if err != nil || !found {
		return BackgroundOperationState{}, found, err
	}
	return backgroundOperationStateFromDatabase(operation), true, nil
}

func getDatabaseBackgroundOperations(client *Client, operationIDs []string) (map[string]BackgroundOperationState, error) {
	stored, err := client.store.GetBackgroundOperations(operationIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]BackgroundOperationState, len(stored))
	for id, operation := range stored {
		result[id] = backgroundOperationStateFromDatabase(operation)
	}
	return result, nil
}

func getDatabaseBackgroundOperationTask(client *Client, operationID string) (BackgroundTaskState, bool, error) {
	task, found, err := client.store.GetBackgroundTaskForOperation(operationID)
	if err != nil || !found {
		return BackgroundTaskState{}, found, err
	}
	return BackgroundTaskState{
		BackgroundTask: backgroundTaskFromDatabase(task),
		Status:         BackgroundWorkStatus(task.Status),
		StartedAt:      task.StartedAt,
	}, true, nil
}

func listDatabaseBackgroundOperations(client *Client, options BackgroundOperationListOptions) ([]BackgroundOperationState, error) {
	operations, err := client.store.ListBackgroundOperationsPage(options.VisibleOnly, options.Limit, options.Offset)
	if err != nil {
		return nil, err
	}
	result := make([]BackgroundOperationState, 0, len(operations))
	for _, operation := range operations {
		result = append(result, backgroundOperationStateFromDatabase(operation))
	}
	return result, nil
}

func cancelDatabaseBackgroundOperation(client *Client, operationID string) (bool, error) {
	return client.store.CancelBackgroundOperation(operationID, time.Now().UTC())
}

func cancelDatabaseBackgroundOperationWithDetails(client *Client, operationID string) (BackgroundOperationCancellation, error) {
	result, err := client.store.CancelBackgroundOperationWithDetails(operationID, time.Now().UTC())
	if err != nil {
		return BackgroundOperationCancellation{}, err
	}
	return BackgroundOperationCancellation{Canceled: result.Canceled, RunningTasks: result.RunningTasks}, nil
}

func cancelDatabaseBackgroundOperationWithCleanupTask(client *Client, operationID, cleanupTaskID string, request BackgroundTaskRequest) (BackgroundOperationCancellation, error) {
	result, err := client.store.CancelBackgroundOperationWithCleanupTask(operationID, time.Now().UTC(), databaseBackgroundTask(cleanupTaskID, request))
	if err != nil {
		return BackgroundOperationCancellation{}, err
	}
	return BackgroundOperationCancellation{Canceled: result.Canceled, RunningTasks: result.RunningTasks}, nil
}

func enqueueDatabaseBackgroundTask(client *Client, q databaseQuerier, id string, request BackgroundTaskRequest) (BackgroundTask, bool, error) {
	task, created, err := client.store.EnqueueBackgroundTask(q, databaseBackgroundTask(id, request))
	if err != nil {
		return BackgroundTask{}, false, err
	}
	return backgroundTaskFromDatabase(task), created, nil
}

func attachDatabaseBackgroundTaskAndRevealOperation(client *Client, operationID string, checkpointJSON []byte, id string, request BackgroundTaskRequest) (BackgroundTask, error) {
	task, err := client.store.AttachBackgroundTaskAndRevealOperation(operationID, checkpointJSON, databaseBackgroundTask(id, request))
	if err != nil {
		return BackgroundTask{}, err
	}
	return backgroundTaskFromDatabase(task), nil
}

func databaseBackgroundTask(id string, request BackgroundTaskRequest) database.NewBackgroundTask {
	return database.NewBackgroundTask{
		ID:                     id,
		OperationID:            request.OperationID,
		DedupeKey:              request.DedupeKey,
		Kind:                   request.Kind,
		SubjectKind:            request.SubjectKind,
		SubjectID:              request.SubjectID,
		InputKey:               request.InputKey,
		ResourceClass:          request.ResourceClass,
		Priority:               request.Priority,
		AvailableAt:            request.AvailableAt,
		MaxAttempts:            request.MaxAttempts,
		TerminalFailureCleanup: databaseBackgroundTaskCleanup(request.TerminalFailureCleanup),
	}
}

func databaseBackgroundTaskCleanup(request *BackgroundTaskCleanupRequest) *database.BackgroundTaskCleanup {
	if request == nil {
		return nil
	}
	return &database.BackgroundTaskCleanup{
		DedupeKey:     request.DedupeKey,
		Kind:          request.Kind,
		SubjectKind:   request.SubjectKind,
		SubjectID:     request.SubjectID,
		InputKey:      request.InputKey,
		ResourceClass: request.ResourceClass,
		Priority:      request.Priority,
		MaxAttempts:   request.MaxAttempts,
	}
}

func cancelDatabaseBackgroundTask(client *Client, taskID string) (bool, error) {
	return client.store.CancelBackgroundTask(taskID, time.Now().UTC())
}

func setDatabaseBackgroundOperationState(client *Client, tx *databaseTx, operationID string, checkpointJSON, resultJSON []byte) error {
	if err := client.store.SetBackgroundOperationCheckpointTx(tx, operationID, checkpointJSON); err != nil {
		return err
	}
	return client.store.SetBackgroundOperationResultTx(tx, operationID, resultJSON)
}

func setDatabaseBackgroundTaskState(client *Client, tx *databaseTx, taskID string, checkpointJSON, resultJSON []byte) error {
	if err := client.store.SetBackgroundTaskCheckpointTx(tx, taskID, checkpointJSON); err != nil {
		return err
	}
	return client.store.SetBackgroundTaskResultTx(tx, taskID, resultJSON)
}

func completeDatabaseBackgroundTaskAttemptTx(client *Client, tx *databaseTx, task BackgroundTask) error {
	if task.ID == "" || task.claimAttempt < 1 {
		return fmt.Errorf("background task claim generation is required")
	}
	return client.store.CompleteBackgroundTaskAttemptTx(tx, task.ID, task.claimAttempt, time.Now().UTC())
}

func backgroundTaskFinalizedByHandlerError() error {
	return background.ErrTaskFinalizedByHandler
}

type backgroundChangeTaskStoreBackend interface {
	RecoverExpiredBackgroundTaskLeases(time.Time) (int, error)
	ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (database.BackgroundTask, bool, error)
	RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error)
	CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error
	FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error)
}

type backgroundChangeTaskStore struct {
	store  backgroundChangeTaskStoreBackend
	notify func()
}

func newBackgroundChangeTaskStore(client *Client) backgroundChangeTaskStore {
	return backgroundChangeTaskStore{store: client.store, notify: client.notifyBackgroundOperationChange}
}

func (s backgroundChangeTaskStore) RecoverExpiredBackgroundTaskLeases(now time.Time) (int, error) {
	recovered, err := s.store.RecoverExpiredBackgroundTaskLeases(now)
	if err == nil && recovered > 0 {
		s.notify()
	}
	return recovered, err
}

func (s backgroundChangeTaskStore) ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (database.BackgroundTask, bool, error) {
	task, claimed, err := s.store.ClaimNextBackgroundTask(resourceClass, workerID, now, leaseDuration)
	if err == nil && claimed {
		s.notify()
	}
	return task, claimed, err
}

func (s backgroundChangeTaskStore) RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error) {
	return s.store.RenewBackgroundTaskLease(taskID, workerID, now, leaseDuration)
}

func (s backgroundChangeTaskStore) CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error {
	err := s.store.CompleteBackgroundTask(taskID, workerID, finishedAt)
	if err == nil {
		s.notify()
	}
	return err
}

func (s backgroundChangeTaskStore) FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error) {
	retrying, err := s.store.FailBackgroundTask(taskID, workerID, finishedAt, retryAt, errorCode, errorMessage)
	if err == nil {
		s.notify()
	}
	return retrying, err
}

func newDatabaseBackgroundRuntime(client *Client, cfg BackgroundWorkerConfig) (BackgroundRuntime, error) {
	handlers := make(map[string]background.Handler, len(cfg.Handlers))
	for kind, handler := range cfg.Handlers {
		handler := handler
		if handler == nil {
			handlers[kind] = nil
			continue
		}
		handlers[kind] = func(ctx context.Context, task database.BackgroundTask) error {
			return handler(ctx, backgroundTaskFromDatabase(task))
		}
	}
	return background.NewRunner(background.RunnerConfig{
		Store:         newBackgroundChangeTaskStore(client),
		ResourceClass: cfg.ResourceClass,
		WorkerID:      cfg.WorkerID,
		Handlers:      handlers,
		LeaseDuration: cfg.LeaseDuration,
		PollInterval:  cfg.PollInterval,
		RetryDelay:    cfg.RetryDelay,
		SubscribeWake: client.SubscribeBackgroundOperationChanges,
	})
}

func backgroundOperationFromDatabase(operation database.BackgroundOperation) BackgroundOperation {
	return BackgroundOperation{
		ID:            operation.ID,
		Kind:          operation.Kind,
		Visible:       operation.Visible,
		ProgressTotal: operation.ProgressTotal,
		CreatedAt:     operation.CreatedAt,
	}
}

func backgroundOperationStateFromDatabase(operation database.BackgroundOperation) BackgroundOperationState {
	return BackgroundOperationState{
		ID:                operation.ID,
		Kind:              operation.Kind,
		Visible:           operation.Visible,
		Status:            BackgroundWorkStatus(operation.Status),
		ProgressTotal:     operation.ProgressTotal,
		ProgressCompleted: operation.ProgressCompleted,
		ProgressFailed:    operation.ProgressFailed,
		CreatedAt:         operation.CreatedAt,
		StartedAt:         operation.StartedAt,
		FinishedAt:        operation.FinishedAt,
		ErrorCode:         operation.ErrorCode,
		ErrorMessage:      operation.ErrorMessage,
	}
}

func backgroundTaskFromDatabase(task database.BackgroundTask) BackgroundTask {
	return BackgroundTask{
		ID:          task.ID,
		OperationID: task.OperationID,
		Kind:        task.Kind,
		SubjectKind: task.SubjectKind,
		SubjectID:   task.SubjectID,
		InputKey:     task.InputKey,
		claimAttempt: task.AttemptCount,
	}
}
