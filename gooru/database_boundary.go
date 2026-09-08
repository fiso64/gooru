package gooru

import (
	"context"

	"gooru.local/internal/background"
	"gooru.local/internal/database"
)

// databaseTx, contentTagPair, and tagFacetExclusion keep database implementation
// types behind the core package's composition boundary. Business-logic files
// should depend on these package-local names rather than importing
// internal/database directly.
type databaseTx = database.Tx
type contentTagPair = database.ContentTagPair
type tagFacetExclusion = database.TagFacetExclusion

var errInvalidSavedSearchOrder = database.ErrInvalidSavedSearchOrder

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
