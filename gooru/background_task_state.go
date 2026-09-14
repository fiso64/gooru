package gooru

import "encoding/json"

// SetBackgroundTaskCheckpoint persists producer-owned restart-recovery state for
// one active task. It is intentionally separate from operation-level state so
// sibling children can recover independently.
func (c *Client) SetBackgroundTaskCheckpoint(taskID string, checkpoint any) error {
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	if err := c.store.SetBackgroundTaskCheckpoint(taskID, encoded); err != nil {
		return err
	}
	c.notifyBackgroundOperationChange()
	return nil
}

// GetBackgroundTaskCheckpoint loads producer-owned restart-recovery state for a
// task, including after a terminal transition when compensation may need it.
func (c *Client) GetBackgroundTaskCheckpoint(taskID string, destination any) (bool, error) {
	encoded, found, err := c.store.GetBackgroundTaskCheckpoint(taskID)
	if err != nil || !found {
		return found, err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, err
	}
	return true, nil
}

// SetBackgroundTaskResult persists a structured success result while a task is
// active. The matching read API only exposes it after completion.
func (c *Client) SetBackgroundTaskResult(taskID string, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := c.store.SetBackgroundTaskResult(taskID, encoded); err != nil {
		return err
	}
	c.notifyBackgroundOperationChange()
	return nil
}

// GetBackgroundTaskResult loads a completed task's structured result.
func (c *Client) GetBackgroundTaskResult(taskID string, destination any) (bool, error) {
	encoded, found, err := c.store.GetBackgroundTaskResult(taskID)
	if err != nil || !found {
		return found, err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, err
	}
	return true, nil
}
