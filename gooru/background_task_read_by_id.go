package gooru

// GetBackgroundTask returns one durable child task by its stable task ID.
// found=false means the task does not exist. This is intentionally read-only so
// transport admission can recognize an idempotent retry without weakening the
// immutable-task checks used when a child is attached to an operation.
func (c *Client) GetBackgroundTask(taskID string) (task BackgroundTaskState, found bool, err error) {
	if taskID == "" {
		return BackgroundTaskState{}, false, nil
	}
	databaseTask, found, err := c.store.GetBackgroundTask(taskID)
	if err != nil || !found {
		return BackgroundTaskState{}, found, err
	}
	return BackgroundTaskState{
		BackgroundTask: backgroundTaskFromDatabase(databaseTask),
		Status:         BackgroundWorkStatus(databaseTask.Status),
		StartedAt:      databaseTask.StartedAt,
	}, true, nil
}
