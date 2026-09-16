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

type backgroundUploadTaskStateStore interface {
	GetBackgroundTaskCheckpoint(string, any) (bool, error)
	SetBackgroundTaskCheckpoint(string, any) error
	SetBackgroundTaskResult(string, any) error
}

type backgroundUploadRecoveryState struct {
	store           backgroundUploadWorkerStore
	taskStore       backgroundUploadTaskStateStore
	operationID     string
	taskID          string
	mirrorOperation bool
}

type backgroundUploadImportState struct {
	operationID string
	taskID      string
}

func (r backgroundUploadRecoveryState) importState() backgroundUploadImportState {
	state := backgroundUploadImportState{}
	if r.taskStore != nil {
		state.taskID = r.taskID
	}
	if r.mirrorOperation {
		state.operationID = r.operationID
	}
	return state
}

func loadBackgroundUploadRecoveryState(store backgroundUploadWorkerStore, task core.BackgroundTask) (backgroundUploadRecoveryState, backgroundUploadCheckpoint, error) {
	operation, found, err := store.GetBackgroundOperation(task.OperationID)
	if err != nil {
		return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, fmt.Errorf("load upload background operation: %w", err)
	}
	if !found {
		return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, errors.New("upload background operation is missing")
	}

	taskStore, hasTaskState := store.(backgroundUploadTaskStateStore)
	multiTask := operation.ProgressTotal > 1
	if multiTask && !hasTaskState {
		return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, errors.New("task-scoped upload recovery store is required for multi-task operation")
	}
	recovery := backgroundUploadRecoveryState{
		store:           store,
		taskStore:       taskStore,
		operationID:     task.OperationID,
		taskID:          task.ID,
		mirrorOperation: !multiTask,
	}

	var checkpoint backgroundUploadCheckpoint
	if hasTaskState {
		found, err := taskStore.GetBackgroundTaskCheckpoint(task.ID, &checkpoint)
		if err != nil {
			return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, fmt.Errorf("load upload task checkpoint: %w", err)
		}
		if found {
			return recovery, checkpoint, nil
		}
		if multiTask {
			return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, errors.New("upload task checkpoint is missing")
		}
	}

	found, err = store.GetBackgroundOperationCheckpoint(task.OperationID, &checkpoint)
	if err != nil {
		return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, fmt.Errorf("load upload background checkpoint: %w", err)
	}
	if !found {
		return backgroundUploadRecoveryState{}, backgroundUploadCheckpoint{}, errors.New("upload background checkpoint is missing")
	}
	return recovery, checkpoint, nil
}

func (r backgroundUploadRecoveryState) setCheckpoint(checkpoint backgroundUploadCheckpoint) error {
	if r.taskStore != nil {
		if err := r.taskStore.SetBackgroundTaskCheckpoint(r.taskID, checkpoint); err != nil {
			return err
		}
		if !r.mirrorOperation {
			return nil
		}
	}
	return r.store.SetBackgroundOperationCheckpoint(r.operationID, checkpoint)
}

func (r backgroundUploadRecoveryState) setResult(result UploadImportResponse) error {
	if r.taskStore != nil {
		if err := r.taskStore.SetBackgroundTaskResult(r.taskID, result); err != nil {
			return err
		}
		if !r.mirrorOperation {
			return nil
		}
	}
	return r.store.SetBackgroundOperationResult(r.operationID, result)
}

type backgroundUploadImporter interface {
	importUploadedFiles(context.Context, []StagedUpload, []string, backgroundUploadImportState) (UploadImportResponse, error)
}

type backgroundUploadResultIdentityEnricher interface {
	populateUploadFileIDs(*UploadImportResponse, []StagedUpload) error
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
	recovery, checkpoint, err := loadBackgroundUploadRecoveryState(store, task)
	if err != nil {
		return err
	}
	switch checkpoint.Phase {
	case backgroundUploadPhaseStaged:
		if err := activateSavedDurableUploads(files); err != nil {
			return err
		}
		checkpoint = backgroundUploadActivatedCheckpoint(len(files), 0)
		if err := recovery.setCheckpoint(checkpoint); err != nil {
			canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
			if stateErr != nil {
				return errors.Join(fmt.Errorf("persist activated upload checkpoint: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
			}
			if canceled {
				if cleanupErr := cleanupCanceledClaimedUpload(files); cleanupErr != nil {
					return cleanupErr
				}
				return nil
			}
			return fmt.Errorf("persist activated upload checkpoint: %w", err)
		}
	case backgroundUploadPhaseActivated:
		if err := restoreDurableNonreplacementDestinations(files); err != nil {
			return fmt.Errorf("restore activated durable uploads: %w", err)
		}
	case backgroundUploadPhaseImported:
		if checkpoint.Response == nil {
			return errors.New("imported upload checkpoint is missing response")
		}
		response := *checkpoint.Response
		response.Files = append([]UploadedFileDTO(nil), response.Files...)
		if enricher, ok := importer.(backgroundUploadResultIdentityEnricher); ok {
			if err := enricher.populateUploadFileIDs(&response, durableStagedUploads(files)); err != nil {
				return fmt.Errorf("restore upload result identities: %w", err)
			}
		}
		if err := settleDurableNonreplacementActivations(files); err != nil {
			return fmt.Errorf("settle imported durable uploads: %w", err)
		}
		if err := recovery.setResult(response); err != nil {
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

	response, err := importer.importUploadedFiles(withDeferredUploadMediaMetadata(ctx), durableStagedUploads(files), tags, recovery.importState())
	if err != nil {
		canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
		if stateErr != nil {
			return errors.Join(err, fmt.Errorf("inspect upload cancellation: %w", stateErr))
		}
		if canceled {
			if cleanupErr := cleanupCanceledClaimedUpload(files); cleanupErr != nil {
				return cleanupErr
			}
			return nil
		}
		return err
	}

	checkpoint = backgroundUploadImportedCheckpoint(response)
	if err := recovery.setCheckpoint(checkpoint); err != nil {
		canceled, stateErr := backgroundUploadOperationCanceled(store, task.OperationID)
		if stateErr != nil {
			return errors.Join(fmt.Errorf("persist imported upload checkpoint: %w", err), fmt.Errorf("inspect upload cancellation: %w", stateErr))
		}
		if canceled {
			if settleErr := settleDurableNonreplacementActivations(files); settleErr != nil {
				return fmt.Errorf("settle canceled durable uploads: %w", settleErr)
			}
			return nil
		}
		return fmt.Errorf("persist imported upload checkpoint: %w", err)
	}
	if err := settleDurableNonreplacementActivations(files); err != nil {
		return fmt.Errorf("settle imported durable uploads: %w", err)
	}
	if err := recovery.setResult(response); err != nil {
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

func cleanupCanceledClaimedUpload(files []savedUpload) error {
	if err := rollbackDurableNonreplacementActivations(files); err != nil {
		return fmt.Errorf("rollback canceled durable uploads: %w", err)
	}
	if err := removeCanceledSavedUploads(files); err != nil {
		return fmt.Errorf("remove canceled staged uploads: %w", err)
	}
	return nil
}
