package gooru

// ListActiveBackgroundOperationIDs returns every pending/running durable
// operation ID. It is intended for action paths such as bulk cancellation; UI
// history callers should continue to use the bounded ListBackgroundOperations.
func (c *Client) ListActiveBackgroundOperationIDs(visibleOnly bool) ([]string, error) {
	return c.store.ListActiveBackgroundOperationIDs(visibleOnly)
}
