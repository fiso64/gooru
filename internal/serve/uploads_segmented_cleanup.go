package serve

import (
	"context"
	"errors"
	"fmt"
	"strings"

	core "gooru.local/gooru"
)

type segmentedDurableUploadCleanupStore interface {
	durableUploadCleanupStore
	GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
	GetBackgroundTask(string) (core.BackgroundTaskState, bool, error)
	GetBackgroundTaskCheckpoint(string, any) (bool, error)
}

type segmentedDurableUploadTaskCheckpointStore struct {
	durableUploadCleanupStore
	checkpointStore segmentedDurableUploadCleanupStore
	taskID          string
}

func (s segmentedDurableUploadTaskCheckpointStore) GetBackgroundOperationCheckpoint(_ string, destination any) (bool, error) {
	return s.checkpointStore.GetBackgroundTaskCheckpoint(s.taskID, destination)
}

// backgroundUploadCleanupHandlerV2 preserves the legacy single-child recovery
// path while recovering each deterministic child independently for segmented
// logical uploads. Operation cancellation recovers every admitted child, while
// terminal-failure compensation recovers only the child that exhausted retries.
func (s *Server) backgroundUploadCleanupHandlerV2(store durableUploadCleanupStore) core.BackgroundTaskHandler {
	legacy := s.backgroundUploadCleanupHandler(store)
	segmented, ok := any(store).(segmentedDurableUploadCleanupStore)
	if !ok {
		return legacy
	}

	return func(ctx context.Context, task core.BackgroundTask) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if task.OperationID != "" || task.Kind != backgroundUploadCleanupTaskKind || task.SubjectKind != "operation" || task.SubjectID == "" {
			return errors.New("upload cleanup task has invalid operation identity")
		}

		operation, found, err := segmented.GetBackgroundOperation(task.SubjectID)
		if err != nil {
			return fmt.Errorf("load canceled upload operation: %w", err)
		}
		if !found || operation.ProgressTotal <= 1 {
			return legacy(ctx, task)
		}

		if sourceTaskID, terminalFailure := strings.CutSuffix(task.ID, ":terminal-cleanup"); terminalFailure {
			child, found, err := segmented.GetBackgroundTask(sourceTaskID)
			if err != nil {
				return fmt.Errorf("load failed upload segment: %w", err)
			}
			if !found {
				return errors.New("failed upload segment task is missing")
			}
			if !durableUploadSegmentTaskMatches(child, task.SubjectID) {
				return errors.New("failed upload segment has invalid durable task identity")
			}
			checkpointStore := segmentedDurableUploadTaskCheckpointStore{
				durableUploadCleanupStore: store,
				checkpointStore:            segmented,
				taskID:                     sourceTaskID,
			}
			if err := cleanupCanceledDurableUpload(checkpointStore, task.SubjectID, child.BackgroundTask); err != nil {
				return fmt.Errorf("cleanup failed upload segment: %w", err)
			}
			return nil
		}

		for segmentIndex := int64(0); segmentIndex < operation.ProgressTotal; segmentIndex++ {
			taskID := durableUploadSegmentTaskID(task.SubjectID, segmentIndex)
			child, found, err := segmented.GetBackgroundTask(taskID)
			if err != nil {
				return fmt.Errorf("load canceled upload segment %d: %w", segmentIndex, err)
			}
			if !found {
				continue
			}
			if !durableUploadSegmentTaskMatches(child, task.SubjectID) {
				return fmt.Errorf("canceled upload segment %d has invalid durable task identity", segmentIndex)
			}

			checkpointStore := segmentedDurableUploadTaskCheckpointStore{
				durableUploadCleanupStore: store,
				checkpointStore:            segmented,
				taskID:                     taskID,
			}
			if err := cleanupCanceledDurableUpload(checkpointStore, task.SubjectID, child.BackgroundTask); err != nil {
				return fmt.Errorf("cleanup canceled upload segment %d: %w", segmentIndex, err)
			}
		}
		return nil
	}
}
