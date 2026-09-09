package gooru

import "encoding/json"

// SetBackgroundOperationResult persists a bounded structured success result while the
// operation is still active. Durable task handlers can call this before successful
// completion; consumers cannot observe the payload until the operation is completed.
func (c *Client) SetBackgroundOperationResult(operationID string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.store.SetBackgroundOperationResult(operationID, encoded)
}

// GetBackgroundOperationResult loads a completed operation's structured success result
// into destination. found=false means there is no completed result available yet.
func (c *Client) GetBackgroundOperationResult(operationID string, destination any) (bool, error) {
	encoded, found, err := c.store.GetBackgroundOperationResult(operationID)
	if err != nil || !found {
		return found, err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, err
	}
	return true, nil
}
