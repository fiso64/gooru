package gooru

// CountActiveBackgroundOperations returns the exact number of pending or
// running durable operations. visibleOnly excludes deliberately hidden
// implementation/background work.
func (c *Client) CountActiveBackgroundOperations(visibleOnly bool) (int, error) {
	return c.store.CountActiveBackgroundOperations(visibleOnly)
}
