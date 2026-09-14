package serve

import (
	"encoding/json"
	"fmt"

	core "gooru.local/gooru"
)

type backgroundUploadSegmentProjectionReader interface {
	GetBackgroundTask(string) (core.BackgroundTaskState, bool, error)
	GetBackgroundTaskCheckpoint(string, any) (bool, error)
}

type backgroundUploadSegmentResultReader interface {
	GetBackgroundTask(string) (core.BackgroundTaskState, bool, error)
	GetBackgroundTaskResult(string, any) (bool, error)
}

func (l *GooruLibrary) GetBackgroundTaskCheckpoint(taskID string, destination any) (bool, error) {
	return l.client.GetBackgroundTaskCheckpoint(taskID, destination)
}

func (l *GooruLibrary) GetBackgroundTaskResult(taskID string, destination any) (bool, error) {
	return l.client.GetBackgroundTaskResult(taskID, destination)
}

func (s *Server) backgroundOperationResult(operation core.BackgroundOperationState, destination any) (bool, error) {
	found, err := s.backgroundOperations.GetBackgroundOperationResult(operation.ID, destination)
	if err != nil || found {
		return found, err
	}
	if operation.Kind == backgroundUploadImportOperationKind && operation.ProgressTotal > 1 {
		if reader, ok := s.backgroundOperations.(backgroundUploadSegmentResultReader); ok {
			return aggregateSegmentedUploadResult(reader, operation, destination)
		}
	}
	return false, nil
}

func aggregateSegmentedUploadResult(reader backgroundUploadSegmentResultReader, operation core.BackgroundOperationState, destination any) (bool, error) {
	response := UploadImportResponse{}
	for segmentIndex := int64(0); segmentIndex < operation.ProgressTotal; segmentIndex++ {
		taskID := durableUploadSegmentTaskID(operation.ID, segmentIndex)
		task, found, err := reader.GetBackgroundTask(taskID)
		if err != nil {
			return false, fmt.Errorf("load upload segment %d: %w", segmentIndex, err)
		}
		if !found {
			return false, fmt.Errorf("completed upload segment %d is missing", segmentIndex)
		}
		if !durableUploadSegmentTaskMatches(task, operation.ID) {
			return false, fmt.Errorf("upload segment %d has invalid task identity", segmentIndex)
		}
		var segment UploadImportResponse
		found, err = reader.GetBackgroundTaskResult(taskID, &segment)
		if err != nil {
			return false, fmt.Errorf("load upload segment %d result: %w", segmentIndex, err)
		}
		if !found {
			return false, fmt.Errorf("completed upload segment %d result is missing", segmentIndex)
		}
		response.Files = append(response.Files, segment.Files...)
		response.AffectedCount += segment.AffectedCount
		response.Notifications = append(response.Notifications, segment.Notifications...)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		return false, fmt.Errorf("encode segmented upload result: %w", err)
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, fmt.Errorf("decode segmented upload result: %w", err)
	}
	return true, nil
}

func (s *Server) segmentedUploadOperationDTO(operation core.BackgroundOperationState, dto BackgroundOperationDTO) (BackgroundOperationDTO, bool) {
	reader, ok := s.backgroundOperations.(backgroundUploadSegmentProjectionReader)
	if !ok || operation.ProgressTotal <= 1 {
		return dto, false
	}

	expected := operation.ProgressTotal
	admitted := int64(0)
	segmentProgress := 0.0
	allFileProgress := true
	totalFiles := int64(0)
	completedFiles := int64(0)
	completedPrefix := int64(0)
	prefixOpen := true

	for segmentIndex := int64(0); segmentIndex < expected; segmentIndex++ {
		taskID := durableUploadSegmentTaskID(operation.ID, segmentIndex)
		task, found, err := reader.GetBackgroundTask(taskID)
		if err != nil {
			return dto, false
		}
		if !found {
			allFileProgress = false
			continue
		}
		if !durableUploadSegmentTaskMatches(task, operation.ID) {
			return dto, false
		}
		admitted++

		var checkpoint backgroundUploadCheckpoint
		found, err = reader.GetBackgroundTaskCheckpoint(taskID, &checkpoint)
		if err != nil {
			return dto, false
		}
		if !found {
			allFileProgress = false
			if task.Status == core.BackgroundWorkCompleted {
				segmentProgress += 1
			}
			continue
		}
		segmentProgress += segmentedUploadCheckpointProgress(task.Status, checkpoint)

		if checkpoint.FileTotal <= 0 {
			allFileProgress = false
			continue
		}
		total := int64(checkpoint.FileTotal)
		completed := int64(checkpoint.FilesCompleted)
		prefix := int64(checkpoint.FilesCompletedPrefix)
		if task.Status == core.BackgroundWorkCompleted || checkpoint.Phase == backgroundUploadPhaseImported {
			completed = total
			prefix = total
		}
		if completed < 0 {
			completed = 0
		}
		if completed > total {
			completed = total
		}
		if prefix < 0 {
			prefix = 0
		}
		if prefix > completed {
			prefix = completed
		}
		totalFiles += total
		completedFiles += completed
		if prefixOpen {
			completedPrefix += prefix
			if prefix < total {
				prefixOpen = false
			}
		}
	}

	if admitted < expected {
		dto.Stage = "receiving"
		if operationReader, ok := s.backgroundOperations.(backgroundOperationCheckpointReader); ok {
			var receiving backgroundUploadCheckpoint
			if found, err := operationReader.GetBackgroundOperationCheckpoint(operation.ID, &receiving); err == nil && found &&
				receiving.Phase == backgroundUploadPhaseReceiving && receiving.TransportBytesTotal > 0 &&
				receiving.TransportBytesReceived > 0 && receiving.TransportBytesReceived < receiving.TransportBytesTotal {
				received := receiving.TransportBytesReceived
				if received > receiving.TransportBytesTotal {
					received = receiving.TransportBytesTotal
				}
				segmentProgress += 0.5 * float64(received) / float64(receiving.TransportBytesTotal)
			}
		}
	} else {
		dto.Stage = "importing"
	}

	progress := segmentProgress / float64(expected)
	if admitted == expected && allFileProgress && totalFiles > 0 {
		dto.ProgressTotal = totalFiles
		dto.ProgressCompleted = completedFiles
		dto.ProgressCompletedPrefix = completedPrefix
		if operation.Status != core.BackgroundWorkFailed && operation.Status != core.BackgroundWorkCanceled {
			dto.ProgressFailed = 0
		}
		progress = 0.5 + 0.5*float64(completedFiles)/float64(totalFiles)
	}
	if operation.Status == core.BackgroundWorkCompleted {
		progress = 1
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	dto.Progress = &progress
	if dto.Status == core.BackgroundWorkPending && (admitted > 0 || progress > 0) {
		dto.Status = core.BackgroundWorkRunning
	}
	return dto, true
}

func segmentedUploadCheckpointProgress(status core.BackgroundWorkStatus, checkpoint backgroundUploadCheckpoint) float64 {
	if status == core.BackgroundWorkCompleted || checkpoint.Phase == backgroundUploadPhaseImported {
		return 1
	}
	switch checkpoint.Phase {
	case backgroundUploadPhaseStaged:
		return 0.5
	case backgroundUploadPhaseActivated:
		if checkpoint.FileTotal <= 0 {
			return 0.5
		}
		completed := checkpoint.FilesCompleted
		if completed < 0 {
			completed = 0
		}
		if completed > checkpoint.FileTotal {
			completed = checkpoint.FileTotal
		}
		return 0.5 + 0.5*float64(completed)/float64(checkpoint.FileTotal)
	default:
		return 0
	}
}
