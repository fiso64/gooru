package gooru

import (
	"errors"
	"fmt"
)

var ErrBackgroundOperationPendingLimit = errors.New("background operation pending limit reached")

// CreateBackgroundOperationWithPendingLimit atomically creates a logical
// operation only when fewer than maxPending operations of the same kind are
// currently pending. Producers can use this before expensive request staging to
// retain bounded queue admission without an in-memory reservation.
func (c *Client) CreateBackgroundOperationWithPendingLimit(request BackgroundOperationRequest, maxPending int) (BackgroundOperation, error) {
	if maxPending < 1 {
		return BackgroundOperation{}, errors.New("background operation pending limit must be positive")
	}
	id, err := newBackgroundWorkID("operation")
	if err != nil {
		return BackgroundOperation{}, err
	}
	operation, admitted, err := c.store.CreateBackgroundOperationWithPendingLimit(
		id,
		request.Kind,
		request.Visible,
		request.ProgressTotal,
		maxPending,
	)
	if err != nil {
		return BackgroundOperation{}, fmt.Errorf("create bounded background operation: %w", err)
	}
	if !admitted {
		return BackgroundOperation{}, ErrBackgroundOperationPendingLimit
	}
	result := backgroundOperationFromDatabase(operation)
	if result.Visible {
		c.notifyBackgroundOperationChange()
	}
	return result, nil
}
