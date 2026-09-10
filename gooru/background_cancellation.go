package gooru

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
	return cancelDatabaseBackgroundOperationWithDetails(c, operationID)
}
