package serve

import (
	"context"
	"errors"
	"fmt"

	core "gooru.local/gooru"
)

type backgroundUploadWorkerStore interface {
	GetBackgroundOperationCheckpoint(string, any) (bool, error)
	SetBackgroundOperationCheckpoint(string, any) error
	SetBackgroundOperationResult(string, any) error
}

func (s *Server) backgroundUploadHandler(store backgroundUploadWorkerStore) core.BackgroundTaskHandler {
	return func(ctx context.Context, task core.BackgroundTask) error {
		importer, ok := s.library.(UploadLibrary)
		if !ok {
			return errors.New("upload import service is not configured")
		}
		return runBackgroundUploadTask(ctx, importer, store, task)
	}
}

func runBackgroundUploadTask(ctx context.Context, importer UploadLibrary, store backgroundUploadWorkerStore, task core.BackgroundTask) error {
	if importer == nil {
		return errors.New("upload import service is not configured")
	}
	if store == nil {
		return errors.New("upload background operation store is not configured")
	}
	files, tags, err := decodeBackgroundUploadTask(task)
	if err != nil {
		return err
	}

	var checkpoint backgroundUploadCheckpoint
	found, err := store.GetBackgroundOperationCheckpoint(task.OperationID, &checkpoint)
	if err != nil {
		return fmt.Errorf("load upload background checkpoint: %w", err)
	}
	if !found {
		return errors.New("upload background checkpoint is missing")
	}

	var activated []activatedSavedReplacement
	switch checkpoint.Phase {
	case backgroundUploadPhaseStaged:
		activated, err = activateSavedReplacements(files)
		if err != nil {
			return err
		}
		checkpoint = backgroundUploadActivatedCheckpoint(activated)
		if err := store.SetBackgroundOperationCheckpoint(task.OperationID, checkpoint); err != nil {
			return fmt.Errorf("persist activated upload checkpoint: %w", err)
		}
	case backgroundUploadPhaseActivated:
		activated, err = activatedSavedReplacementsFromCheckpoint(files, checkpoint)
		if err != nil {
			return err
		}
	case backgroundUploadPhaseImported:
		if checkpoint.Response == nil {
			return errors.New("imported upload checkpoint is missing response")
		}
		activated, err = activatedSavedReplacementsFromCheckpoint(files, checkpoint)
		if err != nil {
			return err
		}
		if err := settleSavedReplacements(activated, *checkpoint.Response); err != nil {
			return fmt.Errorf("settle imported upload replacements: %w", err)
		}
		if err := store.SetBackgroundOperationResult(task.OperationID, *checkpoint.Response); err != nil {
			return fmt.Errorf("publish upload background result: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("upload background checkpoint has invalid phase %q", checkpoint.Phase)
	}

	response, err := importer.ImportUploadedFiles(ctx, stagedUploads(files), tags)
	if err != nil {
		// Keep activated replacement state intact so the durable retry can replay
		// the import against the same filesystem identity. Rolling back here would
		// discard the staged replacement and make a later retry unrecoverable.
		return err
	}
	checkpoint = backgroundUploadImportedCheckpoint(activated, response)
	if err := store.SetBackgroundOperationCheckpoint(task.OperationID, checkpoint); err != nil {
		return fmt.Errorf("persist imported upload checkpoint: %w", err)
	}
	if err := settleSavedReplacements(activated, response); err != nil {
		return fmt.Errorf("settle imported upload replacements: %w", err)
	}
	if err := store.SetBackgroundOperationResult(task.OperationID, response); err != nil {
		return fmt.Errorf("publish upload background result: %w", err)
	}
	return nil
}
