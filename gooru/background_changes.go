package gooru

import "sync"

type backgroundOperationChangeBus struct {
	mu          sync.Mutex
	subscribers map[chan struct{}]struct{}
}

var backgroundOperationChangeBuses sync.Map

func (c *Client) backgroundOperationChangeBus() *backgroundOperationChangeBus {
	candidate := &backgroundOperationChangeBus{subscribers: make(map[chan struct{}]struct{})}
	actual, _ := backgroundOperationChangeBuses.LoadOrStore(c, candidate)
	return actual.(*backgroundOperationChangeBus)
}

// SubscribeBackgroundOperationChanges returns a process-local, payload-free
// signal whenever committed durable-operation state changes. Signals are
// deliberately coalesced per subscriber: consumers must re-read operation state
// rather than treating notifications as an event log.
func (c *Client) SubscribeBackgroundOperationChanges() (<-chan struct{}, func()) {
	bus := c.backgroundOperationChangeBus()
	changes := make(chan struct{}, 1)
	bus.mu.Lock()
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
	value, ok := backgroundOperationChangeBuses.Load(c)
	if !ok {
		return
	}
	bus := value.(*backgroundOperationChangeBus)
	bus.mu.Lock()
	defer bus.mu.Unlock()
	for subscriber := range bus.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
}
