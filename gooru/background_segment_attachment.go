package gooru

import (
	"encoding/json"
	"fmt"
)

// AttachBackgroundTaskToOperation attaches one caller-stable child task and its
// initial recovery checkpoint to an existing logical operation. The stable task
// ID is the idempotency key for transport retries: repeating the same immutable
// task returns created=false without overwriting recovery state that may already
// have advanced. Dedupe keys remain scoped to the parent operation.
func (c *Client) AttachBackgroundTaskToOperation(operationID, taskID string, checkpoint any, request BackgroundTaskRequest) (task BackgroundTask, created bool, err error) {
	if operationID == "" {
		return BackgroundTask{}, false, fmt.Errorf("background operation id is required")
	}
	if taskID == "" {
		return BackgroundTask{}, false, fmt.Errorf("background task id is required")
	}
	if request.OperationID != "" && request.OperationID != operationID {
		return BackgroundTask{}, false, fmt.Errorf("background child task already belongs to operation %q", request.OperationID)
	}
	checkpointJSON, err := json.Marshal(checkpoint)
	if err != nil {
		return BackgroundTask{}, false, fmt.Errorf("encode background task checkpoint: %w", err)
	}
	if request.DedupeKey == "" {
		request.DedupeKey = "task:" + taskID
	}
	request.OperationID = operationID
	request.DedupeKey = operationID + ":" + request.DedupeKey
	return attachDatabaseBackgroundTaskToOperation(c, operationID, checkpointJSON, taskID, request)
}
