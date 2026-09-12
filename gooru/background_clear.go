package gooru

// ClearTerminalBackgroundOperations removes visible terminal operation history
// and its attached terminal task history. Pending/running operations are never
// cleared, and the database layer additionally refuses rows with active children.
func (c *Client) ClearTerminalBackgroundOperations() (int64, error) {
	return c.store.ClearTerminalBackgroundOperations()
}
