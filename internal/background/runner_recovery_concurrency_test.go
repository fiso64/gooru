package background

import (
	"context"
	"sync"
	"testing"
	"time"

	"gooru.local/internal/database"
)

type startupRecoveryConcurrencyStore struct {
	mu      sync.Mutex
	calls   int
	active  int
	max     int
	entered chan int
	release chan struct{}
}

func (s *startupRecoveryConcurrencyStore) RecoverExpiredBackgroundTaskLeases(time.Time) (int, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.active++
	if s.active > s.max {
		s.max = s.active
	}
	s.mu.Unlock()

	s.entered <- call
	<-s.release

	s.mu.Lock()
	s.active--
	s.mu.Unlock()
	return 0, nil
}

func (*startupRecoveryConcurrencyStore) ClaimNextBackgroundTask(string, string, time.Time, time.Duration) (database.BackgroundTask, bool, error) {
	return database.BackgroundTask{}, false, nil
}

func (*startupRecoveryConcurrencyStore) RenewBackgroundTaskLease(string, string, time.Time, time.Duration) (time.Time, error) {
	return time.Time{}, nil
}

func (*startupRecoveryConcurrencyStore) CompleteBackgroundTask(string, string, time.Time) error {
	return nil
}

func (*startupRecoveryConcurrencyStore) FailBackgroundTask(string, string, time.Time, time.Time, string, string) (bool, error) {
	return false, nil
}

func TestRunnerSerializesStartupLeaseRecovery(t *testing.T) {
	store := &startupRecoveryConcurrencyStore{
		entered: make(chan int, 2),
		release: make(chan struct{}),
	}
	newRunner := func(resourceClass, workerID string) *Runner {
		runner, err := NewRunner(RunnerConfig{
			Store:         store,
			ResourceClass: resourceClass,
			WorkerID:      workerID,
			Handlers: map[string]Handler{
				"noop": func(context.Context, database.BackgroundTask) error { return nil },
			},
			PollInterval: time.Millisecond,
		})
		if err != nil {
			t.Fatal(err)
		}
		return runner
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 2)
	go func() { done <- newRunner("media", "worker-media").Run(ctx) }()
	if call := <-store.entered; call != 1 {
		t.Fatalf("first recovery call = %d, want 1", call)
	}

	go func() { done <- newRunner("storage", "worker-storage").Run(ctx) }()
	overlapped := false
	select {
	case <-store.entered:
		overlapped = true
	case <-time.After(100 * time.Millisecond):
	}
	close(store.release)
	if !overlapped {
		if call := <-store.entered; call != 2 {
			t.Fatalf("second recovery call = %d, want 2", call)
		}
	}
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	store.mu.Lock()
	maxActive := store.max
	store.mu.Unlock()
	if overlapped || maxActive != 1 {
		t.Fatalf("startup recovery overlap = %v, max active = %d; want serialized recovery", overlapped, maxActive)
	}
}
