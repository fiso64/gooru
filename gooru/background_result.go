package gooru

import "encoding/json"

// SetBackgroundOperationResult persists a structured producer result while an
// operation is active. The matching read API only exposes it after completion.
func (c *Client) SetBackgroundOperationResult(operationID string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := c.store.SetBackgroundOperationResult(operationID, encoded); err != nil {
		return err
	}
	c.notifyBackgroundOperationChange()
	return nil
}

// GetBackgroundOperationResult loads a completed operation's structured result.
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
