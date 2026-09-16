package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	core "gooru.local/gooru"
)

const (
	backgroundFileRemovalDeleteOperationKind = "files.delete"
	backgroundFileRemovalCleanupTaskKind     = "files.remove.cleanup"
	backgroundFileRemovalCleanupPriority     = 100
)

func backgroundFileRemovalCleanupTaskRequest(operationID string) core.BackgroundTaskRequest {
	return core.BackgroundTaskRequest{
		DedupeKey:     "file-removal-cleanup:" + operationID,
		Kind:          backgroundFileRemovalCleanupTaskKind,
		SubjectKind:   "operation",
		SubjectID:     operationID,
		ResourceClass: backgroundFileRemovalResourceClass,
		Priority:      backgroundFileRemovalCleanupPriority,
		MaxAttempts:   5,
	}
}

func (s *Server) backgroundFileRemovalCleanupHandler(store durableUploadTaskReader) core.BackgroundTaskHandler {
	return func(ctx context.Context, task core.BackgroundTask) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if task.OperationID != "" || task.Kind != backgroundFileRemovalCleanupTaskKind {
			return errors.New("file removal cleanup task has invalid identity")
		}
		if task.InputKey != "" {
			// Terminal failure cleanup embeds the immutable delete payload so the
			// runner can compensate a single failed/expired attempt without replaying
			// the destructive task or needing to discover its operation ID.
			if task.SubjectKind != "file_batch" || task.SubjectID != "selection" {
				return errors.New("terminal file removal cleanup task has invalid identity")
			}
			return s.cleanupCanceledFileRemoval(ctx, core.BackgroundTask{
				Kind:        backgroundFileRemovalTaskKind,
				SubjectKind: task.SubjectKind,
				SubjectID:   task.SubjectID,
				InputKey:    task.InputKey,
			})
		}
		if task.SubjectKind != "operation" || task.SubjectID == "" {
			return errors.New("file removal cleanup task has invalid operation identity")
		}
		original, found, err := store.GetBackgroundOperationTask(task.SubjectID)
		if err != nil {
			return fmt.Errorf("load canceled file removal task: %w", err)
		}
		if !found {
			return errors.New("canceled file removal task is missing")
		}
		if original.BackgroundTask.OperationID != task.SubjectID {
			return errors.New("canceled file removal task has invalid operation identity")
		}
		return s.cleanupCanceledFileRemoval(ctx, original.BackgroundTask)
	}
}

func (s *Server) cleanupCanceledFileRemoval(ctx context.Context, task core.BackgroundTask) error {
	if task.Kind != backgroundFileRemovalTaskKind {
		return errors.New("canceled file removal task has invalid kind")
	}
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(task.InputKey), &envelope); err != nil {
		return fmt.Errorf("decode canceled file removal task envelope: %w", err)
	}
	if envelope.Version == backgroundFileRemovalBatchVersion || envelope.Version == backgroundFileRemovalMixedBatchVersion {
		var input backgroundFileRemovalBatchInput
		if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
			return fmt.Errorf("decode canceled file removal batch task: %w", err)
		}
		expectedMode := "delete"
		managedFiles := input.Files
		if envelope.Version == backgroundFileRemovalMixedBatchVersion {
			expectedMode = "delete_or_untrack"
			managedFiles = make([]backgroundFileRemovalBatchFile, 0, len(input.Files))
			for _, file := range input.Files {
				if file.OriginalPath == "" && file.StagingPath == "" {
					continue
				}
				if file.OriginalPath == "" || file.StagingPath == "" {
					return errors.New("canceled mixed file removal batch has incomplete managed deletion paths")
				}
				managedFiles = append(managedFiles, file)
			}
		}
		if task.SubjectKind != "file_batch" || task.SubjectID != "selection" || input.Mode != expectedMode || len(input.Files) == 0 {
			return errors.New("canceled file removal batch task has invalid identity")
		}
		var cleanupErr error
		for _, file := range managedFiles {
			if err := s.cleanupCanceledFlatFileRemoval(ctx, file); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("reconcile canceled file removal %q: %w", file.PublicID, err))
			}
		}
		return cleanupErr
	}

	var input backgroundFileRemovalInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		return fmt.Errorf("decode canceled file removal task: %w", err)
	}
	if task.SubjectKind != "file" || task.SubjectID == "" || task.SubjectID != input.PublicID || input.Mode != "delete" {
		return errors.New("canceled file removal task has invalid file identity")
	}
	return s.cleanupCanceledLegacyFileRemoval(ctx, input)
}

func (s *Server) cleanupCanceledFlatFileRemoval(ctx context.Context, input backgroundFileRemovalBatchFile) error {
	if input.PublicID == "" {
		return errors.New("file removal batch contains a file without public identity")
	}
	staged, err := persistedFlatStagedFileDeletion(input.OriginalPath, input.StagingPath)
	if err != nil {
		return err
	}
	return s.reconcileCanceledStagedFileRemoval(ctx, input.PublicID, staged.originalPath, staged.stagedPath, staged.rollbackMissingOK, staged.commitMissingOK)
}

func (s *Server) cleanupCanceledLegacyFileRemoval(ctx context.Context, input backgroundFileRemovalInput) error {
	staged, err := persistedStagedFileDeletion(input.OriginalPath, input.StagingPath)
	if err != nil {
		return err
	}
	return s.reconcileCanceledStagedFileRemoval(ctx, input.PublicID, staged.originalPath, staged.stagedPath, staged.rollbackMissingOK, staged.commitMissingOK)
}

func (s *Server) reconcileCanceledStagedFileRemoval(ctx context.Context, publicID, originalPath, stagedPath string, restore, purge func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.validatePersistedDeletionParent(originalPath); err != nil {
		return err
	}
	stagedExists, err := safeStagedDeletionEntryExists(stagedPath)
	if err != nil {
		return fmt.Errorf("check canceled file removal staging path: %w", err)
	}
	if !stagedExists {
		return nil
	}

	file, err := s.getFileByPublicID(ctx, publicID)
	if errors.Is(err, ErrNotFound) {
		if err := purge(); err != nil {
			return fmt.Errorf("purge committed canceled file removal: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve canceled file removal: %w", err)
	}
	managedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))
	if !ok {
		return ErrFileNotManaged
	}
	if filepath.Clean(managedPath) != filepath.Clean(originalPath) {
		return errors.New("managed file path changed after deletion was queued")
	}
	if err := restore(); err != nil {
		return fmt.Errorf("restore canceled staged file: %w", err)
	}
	return nil
}

type durableFileRemovalCancellationStore interface {
	GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
	GetBackgroundOperationTask(string) (core.BackgroundTaskState, bool, error)
	CancelBackgroundOperationWithCleanupTask(string, core.BackgroundTaskRequest) (core.BackgroundOperationCancellation, error)
}

type durableFileRemovalOperationStore struct {
	*GooruLibrary
}

func (s *durableFileRemovalOperationStore) CancelBackgroundOperation(operationID string) (bool, error) {
	handled, canceled, err := cancelDurableFileRemovalOperation(s.GooruLibrary, operationID)
	if handled || err != nil {
		return canceled, err
	}
	return s.GooruLibrary.CancelBackgroundOperation(operationID)
}

func cancelDurableFileRemovalOperation(store durableFileRemovalCancellationStore, operationID string) (handled, canceled bool, err error) {
	state, found, err := store.GetBackgroundOperation(operationID)
	if err != nil {
		return true, false, err
	}
	if !found || state.Kind != backgroundFileRemovalDeleteOperationKind {
		return false, false, nil
	}
	task, found, err := store.GetBackgroundOperationTask(operationID)
	if err != nil {
		return true, false, err
	}
	if !found {
		return false, false, nil
	}
	if task.BackgroundTask.Kind != backgroundFileRemovalTaskKind {
		return true, false, errors.New("file removal operation has invalid background task kind")
	}
	result, err := store.CancelBackgroundOperationWithCleanupTask(operationID, backgroundFileRemovalCleanupTaskRequest(operationID))
	return true, result.Canceled, err
}
