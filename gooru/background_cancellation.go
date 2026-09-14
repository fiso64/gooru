package gooru

import "errors"

// BackgroundOperationCancellation describes the ownership state at the exact
// durable cancellation transaction. RunningTasks is the number of child tasks
// that held a worker lease when cancellation won; producers with external side
// effects can use it to avoid racing cleanup against a worker that is unwinding.
type BackgroundOperationCancellation struct {
	Canceled     bool
	RunningTasks int64
}

// CancelBackgroundOperationWithDetails durably cancels an active logical
// operation and reports whether any child worker still owns in-flight cleanup.
// Callers that do not own external side effects should use CancelBackgroundOperation.
func (c *Client) CancelBackgroundOperationWithDetails(operationID string) (BackgroundOperationCancellation, error) {
	result, err := cancelDatabaseBackgroundOperationWithDetails(c, operationID)
	if err == nil && result.Canceled {
		c.notifyBackgroundOperationChange()
	}
	return result, err
}

// CancelBackgroundOperationWithCleanupTask durably cancels an active logical
// operation and atomically creates one detached follow-up task. The persistence
// layer delays that task through any revoked child-worker lease horizon, making
// it suitable for replayable compensation of external side effects after cancel.
func (c *Client) CancelBackgroundOperationWithCleanupTask(operationID string, request BackgroundTaskRequest) (BackgroundOperationCancellation, error) {
	if request.OperationID != "" {
		return BackgroundOperationCancellation{}, errors.New("background cancellation cleanup task must be detached from an operation")
	}
	id, err := newBackgroundWorkID("task")
	if err != nil {
		return BackgroundOperationCancellation{}, err
	}
	result, err := cancelDatabaseBackgroundOperationWithCleanupTask(c, operationID, id, request)
	if err == nil && result.Canceled {
		c.notifyBackgroundOperationChange()
	}
	return result, err
}
