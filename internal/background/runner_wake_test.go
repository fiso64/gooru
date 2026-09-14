package background

import (
	"context"
	"sync"
	"testing"
	"time"

	"gooru.local/internal/database"
)

type wakeAwareFakeTaskStore struct {
	*fakeTaskStore
	emptyOnce sync.Once
	empty     chan struct{}
}

func (s *wakeAwareFakeTaskStore) ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (database.BackgroundTask, bool, error) {
	task, ok, err := s.fakeTaskStore.ClaimNextBackgroundTask(resourceClass, workerID, now, leaseDuration)
	if err == nil && !ok {
		s.emptyOnce.Do(func() { close(s.empty) })
	}
	return task, ok, err
}

func TestRunnerWakesForNewTaskBeforeLongPollInterval(t *testing.T) {
	base := &fakeTaskStore{}
	store := &wakeAwareFakeTaskStore{
		fakeTaskStore: base,
		empty:         make(chan struct{}),
	}
	wake := make(chan struct{}, 1)
	handled := make(chan struct{})

	runner, err := NewRunner(RunnerConfig{
		Store:         store,
		ResourceClass: "image",
		WorkerID:      "worker-a",
		Handlers: map[string]Handler{
			"thumbnail": func(context.Context, database.BackgroundTask) error {
				close(handled)
				return nil
			},
		},
		LeaseDuration: time.Second,
		PollInterval:  time.Hour,
		RetryDelay:    time.Second,
		SubscribeWake: func() (<-chan struct{}, func()) {
			return wake, func() {}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx) }()

	select {
	case <-store.empty:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("runner did not observe an empty queue")
	}

	base.mu.Lock()
	base.claimed = append(base.claimed, database.BackgroundTask{ID: "task-wake", Kind: "thumbnail"})
	base.mu.Unlock()
	wake <- struct{}{}

	select {
	case <-handled:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("runner did not wake before the polling interval")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not stop after cancellation")
	}
}
