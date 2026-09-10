package serve

import (
	"context"
	"errors"
	"fmt"

	core "gooru.local/gooru"
)

type backgroundUploadWorkerStore interface {
	GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
	GetBackgroundOperationCheckpoint(string, any) (bool, error)
	SetBackgroundOperationCheckpoint(string, any) error
	SetBackgroundOperationResult(string, any) error
}

// backgroundUploadImporter is deliberately narrower than UploadLibrary: durable
// execution must use the operation-aware import transaction so content/tag
// registration and the imported checkpoint/result cannot diverge across a crash.
// hiddenTagLibrary embeds GooruLibrary, so it inherits this package-local method
// without exposing durable-work details outside serve.
type backgroundUploadImporter interface {
	importUploadedFiles(context.Context, []StagedUpload, []string, string, []activatedSavedReplacement) (UploadImportResponse, error)
}

func (s *Server) backgroundUploadHandler(store backgroundUploadWorkerStore) core.BackgroundTaskHandler {
	return func(ctx context.Context, task core.BackgroundTask) error {
		importer, ok := s.library.(backgroundUploadImporter)
		if !ok {
			return errors.New("durable upload import service is not configured")
		}
		return runBackgroundUploadTask(ctx, importer, store, task)
	}
}

func runBackgroundUploadTask(ctx context.Context, importer backgroundUploadImporter, store backgroundUploadWorkerStore, task core.BackgroundTask) error {
	if importer == nil {
		return errors.New("durable upload import service is not configured")
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
			canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
			if stateErr != nil {
				return errors.Join(fmt.Errorf("persist activated upload checkpoint: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
			}
			if canceled {
				if cleanupErr := cleanupCanceledClaimedUpload(files, activated); cleanupErr != nil {
					return cleanupErr
				}
				return nil
			}
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
			canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
			if stateErr != nil {
				return errors.Join(fmt.Errorf("publish upload background result: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
			}
			if canceled {
				return nil
			}
			return fmt.Errorf("publish upload background result: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("upload background checkpoint has invalid phase %q", checkpoint.Phase)
	}

	response, err := importer.importUploadedFiles(ctx, stagedUploads(files), tags, task.OperationID, activated)
	if err != nil {
		canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
		if stateErr != nil {
			return errors.Join(err, fmt.Errorf("inspect upload cancellation: %w", stateErr))
		}
		if canceled {
			if cleanupErr := cleanupCanceledClaimedUpload(files, activated); cleanupErr != nil {
				return cleanupErr
			}
			return nil
		}
		// Keep activated replacement state intact for a real retry. Process
		// shutdown and transient failures leave the operation active, while a
		// terminal user cancellation is handled above.
		return err
	}

	// For real imported locations, importUploadedFiles persists this checkpoint
	// and result atomically with content/tag registration. Repeating the checkpoint
	// here is intentional: all-rejected/skipped batches have no domain mutation
	// transaction and still need a replayable terminal phase.
	checkpoint = backgroundUploadImportedCheckpoint(activated, response)
	if err := store.SetBackgroundOperationCheckpoint(task.OperationID, checkpoint); err != nil {
		canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
		if stateErr != nil {
			return errors.Join(fmt.Errorf("persist imported upload checkpoint: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
		}
		if canceled {
			// A cancellation can only win here when the import did not atomically
			// persist a success result (for example, an all-skipped/duplicate batch).
			// Settle replacements according to the completed import response; rolling
			// all of them back blindly could undo a committed imported replacement.
			if settleErr := settleSavedReplacements(activated, response); settleErr != nil {
				return fmt.Errorf("settle canceled upload replacements: %w", settleErr)
			}
			return nil
		}
		return fmt.Errorf("persist imported upload checkpoint: %w", err)
	}
	if err := settleSavedReplacements(activated, response); err != nil {
		return fmt.Errorf("settle imported upload replacements: %w", err)
	}
	if err := store.SetBackgroundOperationResult(task.OperationID, response); err != nil {
		canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
		if stateErr != nil {
			return errors.Join(fmt.Errorf("publish upload background result: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
		}
		if canceled {
			return nil
		}
		return fmt.Errorf("publish upload background result: %w", err)
	}
	return nil
}

func backgroundUploadOperationCanceled(store backgroundUploadWorkerStore, operationID string) (bool, error) {
	state, found, err := store.GetBackgroundOperation(operationID)
	if err != nil || !found {
		return false, err
	}
	return state.Status == core.BackgroundWorkCanceled, nil
}

func cleanupCanceledClaimedUpload(files []savedUpload, activated []activatedSavedReplacement) error {
	if err := rollbackSavedReplacements(activated); err != nil {
		return fmt.Errorf("rollback canceled upload replacements: %w", err)
	}
	removeSavedUploads(files)
	return nil
}
