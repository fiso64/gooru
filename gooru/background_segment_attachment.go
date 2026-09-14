package gooru

import (
	"encoding/json"
	"fmt"
)

// AttachBackgroundTaskToOperation attaches one caller-stable child task and its
// initial recovery checkpoint to an existing logical operation. The stable task
// ID is the idempotency identity for transport retries: repeating the same
// immutable task returns created=false without overwriting recovery state that
// may already have advanced. The operation-scoped dedupe key is derived from
// that stable task ID so sibling children can safely share a logical request
// dedupe key such as "import" without colliding with one another.
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
	request.OperationID = operationID
	request.DedupeKey = operationID + ":task:" + taskID
	return attachDatabaseBackgroundTaskToOperation(c, operationID, checkpointJSON, taskID, request)
}
