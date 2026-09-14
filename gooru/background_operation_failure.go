package gooru

import "errors"

// BackgroundOperationFailure describes whether a producer-side terminal failure
// won and how many child workers still held a lease at that transaction.
type BackgroundOperationFailure struct {
	Failed       bool
	RunningTasks int64
}

// FailBackgroundOperation records a producer-side failure for an active logical
// operation and revokes active child work.
func (c *Client) FailBackgroundOperation(operationID, errorCode, errorMessage string) (bool, error) {
	failed, err := failDatabaseBackgroundOperation(c, operationID, errorCode, errorMessage)
	if err == nil && failed {
		c.notifyBackgroundOperationChange()
	}
	return failed, err
}

// FailBackgroundOperationWithCleanupTask records producer failure and atomically
// creates a detached cleanup obligation when the operation has child work.
func (c *Client) FailBackgroundOperationWithCleanupTask(operationID, errorCode, errorMessage string, request BackgroundTaskRequest) (BackgroundOperationFailure, error) {
	if request.OperationID != "" {
		return BackgroundOperationFailure{}, errors.New("background failure cleanup task must be detached from an operation")
	}
	id, err := newBackgroundWorkID("task")
	if err != nil {
		return BackgroundOperationFailure{}, err
	}
	result, err := failDatabaseBackgroundOperationWithCleanupTask(c, operationID, errorCode, errorMessage, id, request)
	if err == nil && result.Failed {
		c.notifyBackgroundOperationChange()
	}
	return result, err
}
