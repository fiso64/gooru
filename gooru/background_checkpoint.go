package gooru

import "encoding/json"

// SetBackgroundOperationCheckpoint persists internal restart-recovery state for
// an active operation. Checkpoints are not part of the user-visible operation
// contract; producers own their schema and lifecycle.
func (c *Client) SetBackgroundOperationCheckpoint(operationID string, checkpoint any) error {
	encoded, err := json.Marshal(checkpoint)
	if err != nil {
		return err
	}
	return c.store.SetBackgroundOperationCheckpoint(operationID, encoded)
}

// GetBackgroundOperationCheckpoint loads producer-owned restart-recovery state.
func (c *Client) GetBackgroundOperationCheckpoint(operationID string, destination any) (bool, error) {
	encoded, found, err := c.store.GetBackgroundOperationCheckpoint(operationID)
	if err != nil || !found {
		return found, err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, err
	}
	return true, nil
}

// SetBackgroundOperationVisible changes whether an operation is exposed through
// user-facing operation history. Bounded producers use a hidden operation as an
// admission reservation, then reveal it only after durable work is attached.
func (c *Client) SetBackgroundOperationVisible(operationID string, visible bool) error {
	return c.store.SetBackgroundOperationVisible(operationID, visible)
}

// CancelUnattachedHiddenBackgroundOperations releases pre-crash admission
// reservations before producers begin accepting requests after startup.
func (c *Client) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	return c.store.CancelUnattachedHiddenBackgroundOperations(kind)
}

// CancelUnattachedBackgroundOperations releases pre-crash producer work that
// was already visible before its durable child task could be attached.
func (c *Client) CancelUnattachedBackgroundOperations(kind string) (int64, error) {
	return c.store.CancelUnattachedBackgroundOperations(kind)
}

// CancelUnattachedBackgroundOperationIDs releases pre-crash producer work and
// returns the exact operation identities whose external staging state can be
// reclaimed by the producer during startup.
func (c *Client) CancelUnattachedBackgroundOperationIDs(kind string) ([]string, error) {
	return c.store.CancelUnattachedBackgroundOperationIDs(kind)
}
