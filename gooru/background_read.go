package gooru

import "time"

// BackgroundWorkStatus is the durable lifecycle visible to operation consumers.
type BackgroundWorkStatus string

const (
	BackgroundWorkPending   BackgroundWorkStatus = "pending"
	BackgroundWorkRunning   BackgroundWorkStatus = "running"
	BackgroundWorkCompleted BackgroundWorkStatus = "completed"
	BackgroundWorkFailed    BackgroundWorkStatus = "failed"
	BackgroundWorkCanceled  BackgroundWorkStatus = "canceled"
)

// BackgroundOperationState is the read model for one logical durable operation.
// Unlike BackgroundOperation, which is a creation receipt, this type includes
// mutable lifecycle/progress/error state intended for jobs UI and diagnostics.
type BackgroundOperationState struct {
	ID                string
	Kind              string
	Visible           bool
	Status            BackgroundWorkStatus
	ProgressTotal     int64
	ProgressCompleted int64
	ProgressFailed    int64
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	ErrorCode         string
	ErrorMessage      string
}

// BackgroundTaskState is the read model for one durable child task. The embedded
// BackgroundTask remains the narrow handler input while lifecycle fields let
// orchestration code distinguish never-claimed work from a running owner.
type BackgroundTaskState struct {
	BackgroundTask
	Status    BackgroundWorkStatus
	StartedAt *time.Time
}

// BackgroundOperationListOptions controls bounded operation-history reads.
type BackgroundOperationListOptions struct {
	// VisibleOnly omits deliberately hidden implementation/background operations.
	VisibleOnly bool
	// Limit defaults to 100 when non-positive and is capped at 1000.
	Limit int
	// Offset skips the newest matching operations. Negative values are treated as zero.
	Offset int
}

// GetBackgroundOperation returns the latest durable state for one logical
// operation. found=false means the operation does not exist.
func (c *Client) GetBackgroundOperation(operationID string) (operation BackgroundOperationState, found bool, err error) {
	return getDatabaseBackgroundOperation(c, operationID)
}

// GetBackgroundOperations returns the latest durable states for the requested
// operation IDs. Missing IDs are omitted from the result.
func (c *Client) GetBackgroundOperations(operationIDs []string) (map[string]BackgroundOperationState, error) {
	return getDatabaseBackgroundOperations(c, operationIDs)
}

// GetBackgroundOperationTask returns the first durable child task attached to
// an operation. found=false means no child has been attached.
func (c *Client) GetBackgroundOperationTask(operationID string) (task BackgroundTaskState, found bool, err error) {
	return getDatabaseBackgroundOperationTask(c, operationID)
}

// ListBackgroundOperations returns newest-first durable operation state. It is
// intentionally bounded so normal UI/diagnostic callers cannot materialize an
// unbounded history in memory.
func (c *Client) ListBackgroundOperations(options BackgroundOperationListOptions) ([]BackgroundOperationState, error) {
	return listDatabaseBackgroundOperations(c, options)
}

// CountBackgroundOperations returns the exact number of durable operations.
// visibleOnly excludes deliberately hidden implementation/background work.
func (c *Client) CountBackgroundOperations(visibleOnly bool) (int, error) {
	return c.store.CountBackgroundOperations(visibleOnly)
}
