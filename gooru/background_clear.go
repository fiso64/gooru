package gooru

// ClearTerminalBackgroundOperations removes completed, failed, and canceled
// operation history while leaving active durable work untouched.
func (c *Client) ClearTerminalBackgroundOperations() (int64, error) {
	cleared, err := c.store.ClearTerminalBackgroundOperations()
	if err == nil && cleared > 0 {
		c.notifyBackgroundOperationChange()
	}
	return cleared, err
}
