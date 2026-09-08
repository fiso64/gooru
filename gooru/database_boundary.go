package gooru

import (
	"context"

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
