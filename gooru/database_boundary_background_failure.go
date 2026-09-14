package gooru

import (
	"time"

	"gooru.local/internal/database"
)

func failDatabaseBackgroundOperation(client *Client, operationID, errorCode, errorMessage string) (bool, error) {
	return client.store.FailBackgroundOperation(operationID, time.Now().UTC(), errorCode, errorMessage)
}

func failDatabaseBackgroundOperationWithCleanupTask(client *Client, operationID, errorCode, errorMessage, cleanupTaskID string, request BackgroundTaskRequest) (BackgroundOperationFailure, error) {
	result, err := client.store.FailBackgroundOperationWithCleanupTask(operationID, time.Now().UTC(), errorCode, errorMessage, databaseBackgroundFailureTask(cleanupTaskID, request))
	if err != nil {
		return BackgroundOperationFailure{}, err
	}
	return BackgroundOperationFailure{Failed: result.Failed, RunningTasks: result.RunningTasks}, nil
}

func databaseBackgroundFailureTask(id string, request BackgroundTaskRequest) database.NewBackgroundTask {
	return database.NewBackgroundTask{
		ID:                     id,
		OperationID:            request.OperationID,
		DedupeKey:              request.DedupeKey,
		Kind:                   request.Kind,
		SubjectKind:            request.SubjectKind,
		SubjectID:              request.SubjectID,
		InputKey:               request.InputKey,
		ResourceClass:          request.ResourceClass,
		Priority:               request.Priority,
		AvailableAt:            request.AvailableAt,
		MaxAttempts:            request.MaxAttempts,
		TerminalFailureCleanup: databaseBackgroundTaskCleanup(request.TerminalFailureCleanup),
	}
}
