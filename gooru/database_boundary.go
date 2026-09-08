package gooru

import (
	"context"
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

func getDatabaseBackgroundOperation(client *Client, operationID string) (BackgroundOperationState, bool, error) {
	operation, found, err := client.store.GetBackgroundOperation(operationID)
	if err != nil || !found {
		return BackgroundOperationState{}, found, err
	}
	return backgroundOperationStateFromDatabase(operation), true, nil
}

func listDatabaseBackgroundOperations(client *Client, options BackgroundOperationListOptions) ([]BackgroundOperationState, error) {
	operations, err := client.store.ListBackgroundOperations(options.VisibleOnly, options.Limit)
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

func enqueueDatabaseBackgroundTask(client *Client, q databaseQuerier, id string, request BackgroundTaskRequest) (BackgroundTask, bool, error) {
	task, created, err := client.store.EnqueueBackgroundTask(q, database.NewBackgroundTask{
		ID:            id,
		OperationID:   request.OperationID,
		DedupeKey:     request.DedupeKey,
		Kind:          request.Kind,
		SubjectKind:   request.SubjectKind,
		SubjectID:     request.SubjectID,
		InputKey:      request.InputKey,
		ResourceClass: request.ResourceClass,
		Priority:      request.Priority,
		AvailableAt:   request.AvailableAt,
		MaxAttempts:   request.MaxAttempts,
	})
	if err != nil {
		return BackgroundTask{}, false, err
	}
	return backgroundTaskFromDatabase(task), created, nil
}

func cancelDatabaseBackgroundTask(client *Client, taskID string) (bool, error) {
	return client.store.CancelBackgroundTask(taskID, time.Now().UTC())
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
		Store:         client.store,
		ResourceClass: cfg.ResourceClass,
		WorkerID:      cfg.WorkerID,
		Handlers:      handlers,
		LeaseDuration: cfg.LeaseDuration,
		PollInterval:  cfg.PollInterval,
		RetryDelay:    cfg.RetryDelay,
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
		InputKey:    task.InputKey,
	}
}
