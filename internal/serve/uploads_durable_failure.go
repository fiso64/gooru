package serve

import core "gooru.local/gooru"

type durableUploadFailureStore interface {
	FailBackgroundOperationWithCleanupTask(string, string, string, core.BackgroundTaskRequest) (core.BackgroundOperationFailure, error)
}

func (l *GooruLibrary) FailBackgroundOperationWithCleanupTask(operationID, errorCode, errorMessage string, request core.BackgroundTaskRequest) (core.BackgroundOperationFailure, error) {
	return l.client.FailBackgroundOperationWithCleanupTask(operationID, errorCode, errorMessage, request)
}

// failDurableUploadProducer records a producer-side failure without conflating it
// with user cancellation. The fallback keeps test/minimal implementations of the
// older operation boundary working; production GooruLibrary implements the
// failure store and therefore never uses cancellation for this path.
func failDurableUploadProducer(operations durableUploadOperationStore, operationID string) {
	if failures, ok := any(operations).(durableUploadFailureStore); ok {
		_, _ = failures.FailBackgroundOperationWithCleanupTask(
			operationID,
			"upload_request_failed",
			"upload request failed before durable import",
			backgroundUploadCleanupTaskRequest(operationID),
		)
		return
	}
	_, _ = operations.CancelBackgroundOperation(operationID)
}
