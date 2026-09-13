package serve

import (
	"fmt"
)

func durableUploadPriorSegmentDestinations(store durableUploadSegmentStore, operationID string, segmentIndex int64) (map[string]struct{}, error) {
	reserved := make(map[string]struct{})
	for priorIndex := int64(0); priorIndex < segmentIndex; priorIndex++ {
		taskID := durableUploadSegmentTaskID(operationID, priorIndex)
		task, found, err := store.GetBackgroundTask(taskID)
		if err != nil {
			return nil, fmt.Errorf("load prior upload segment %d: %w", priorIndex, err)
		}
		if !found {
			continue
		}
		if !durableUploadSegmentTaskMatches(task, operationID) {
			return nil, fmt.Errorf("prior upload segment %d has invalid durable task identity", priorIndex)
		}
		files, _, err := decodeBackgroundUploadTask(task.BackgroundTask)
		if err != nil {
			return nil, fmt.Errorf("decode prior upload segment %d: %w", priorIndex, err)
		}
		for _, file := range files {
			if file.destinationPath != "" {
				reserved[file.destinationPath] = struct{}{}
			}
		}
	}
	return reserved, nil
}
