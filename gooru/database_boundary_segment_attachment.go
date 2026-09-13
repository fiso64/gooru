package gooru

func attachDatabaseBackgroundTaskToOperation(client *Client, operationID string, checkpointJSON []byte, id string, request BackgroundTaskRequest) (BackgroundTask, bool, error) {
	task, created, err := client.store.AttachBackgroundTaskToOperation(operationID, checkpointJSON, databaseBackgroundTask(id, request))
	if err != nil {
		return BackgroundTask{}, false, err
	}
	if created {
		client.notifyBackgroundOperationChange()
	}
	return backgroundTaskFromDatabase(task), created, nil
}
