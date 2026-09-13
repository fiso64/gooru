package gooru

import "sync"

type backgroundOperationChangeBus struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

func (c *Client) backgroundOperationChangeBus() *backgroundOperationChangeBus {
	return &c.backgroundOperationChanges
}

// SubscribeBackgroundOperationChanges returns a process-local, payload-free
// signal whenever committed durable-operation state changes. Signals are
// deliberately coalesced per subscriber: consumers must re-read operation state
// rather than treating notifications as an event log.
func (c *Client) SubscribeBackgroundOperationChanges() (<-chan struct{}, func()) {
	bus := c.backgroundOperationChangeBus()
	changes := make(chan struct{}, 1)
	bus.mu.Lock()
	if bus.subscribers == nil {
		bus.subscribers = make(map[chan struct{}]struct{})
	}
	bus.subscribers[changes] = struct{}{}
	bus.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			bus.mu.Lock()
			delete(bus.subscribers, changes)
			close(changes)
			bus.mu.Unlock()
		})
	}
	return changes, cancel
}

func (c *Client) notifyBackgroundOperationChange() {
	bus := c.backgroundOperationChangeBus()
	bus.mu.Lock()
	defer bus.mu.Unlock()
	for subscriber := range bus.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
