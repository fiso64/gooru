package gooru

import "time"

func failDatabaseBackgroundOperation(client *Client, operationID, errorCode, errorMessage string) (bool, error) {
	return client.store.FailBackgroundOperation(operationID, time.Now().UTC(), errorCode, errorMessage)
}

func failDatabaseBackgroundOperationWithCleanupTask(client *Client, operationID, errorCode, errorMessage, cleanupTaskID string, request BackgroundTaskRequest) (BackgroundOperationFailure, error) {
	result, err := client.store.FailBackgroundOperationWithCleanupTask(operationID, time.Now().UTC(), errorCode, errorMessage, databaseBackgroundTask(cleanupTaskID, request))
	if err != nil {
		return BackgroundOperationFailure{}, err
	}
	return BackgroundOperationFailure{Failed: result.Failed, RunningTasks: result.RunningTasks}, nil
}
