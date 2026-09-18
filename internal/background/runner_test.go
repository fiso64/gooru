package background

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gooru.local/internal/database"
)

type fakeTaskStore struct {
	mu sync.Mutex

	recovered int
	claimed   []database.BackgroundTask
	renewed   int
	completed []string
	failed    []failedTask

	recoverErr  error
	claimErr    error
	renewErr    error
	completeErr error
	failErr     error
	renewedCh   chan struct{}
}

type failedTask struct {
	id      string
	code    string
	message string
	retryAt time.Time
}

func (s *fakeTaskStore) RecoverExpiredBackgroundTaskLeases(time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recovered++
	return 0, s.recoverErr
}

func (s *fakeTaskStore) ClaimNextBackgroundTask(string, string, time.Time, time.Duration) (database.BackgroundTask, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimErr != nil {
		return database.BackgroundTask{}, false, s.claimErr
	}
	if len(s.claimed) == 0 {
		return database.BackgroundTask{}, false, nil
	}
	task := s.claimed[0]
	s.claimed = s.claimed[1:]
	return task, true, nil
}

func (s *fakeTaskStore) RenewBackgroundTaskLease(string, string, time.Time, time.Duration) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renewed++
	if s.renewedCh != nil && s.renewed == 1 {
		close(s.renewedCh)
	}
	return time.Now(), s.renewErr
}

func (s *fakeTaskStore) CompleteBackgroundTask(id, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed = append(s.completed, id)
	return s.completeErr
}

func (s *fakeTaskStore) FailBackgroundTask(id, _ string, _ time.Time, retryAt time.Time, code, message string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = append(s.failed, failedTask{id: id, code: code, message: message, retryAt: retryAt})
	return true, s.failErr
}

func newTestRunner(t *testing.T, store *fakeTaskStore, handlers map[string]Handler) *Runner {
	t.Helper()
	runner, err := NewRunner(RunnerConfig{
		Store:         store,
		ResourceClass: "image",
		WorkerID:      "worker-a",
		Handlers:      handlers,
		LeaseDuration: 30 * time.Millisecond,
		PollInterval:  time.Millisecond,
		RetryDelay:    2 * time.Second,
		Now:           func() time.Time { return time.Date(2026, 9, 7, 23, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestRunnerRecoversExpiredLeasesBeforePolling(t *testing.T) {
	store := &fakeTaskStore{}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(context.Context, database.BackgroundTask) error { return nil }})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.recovered != 1 {
		t.Fatalf("recover calls = %d, want 1", store.recovered)
	}
}

func TestRunnerCompletesSuccessfulTask(t *testing.T) {
	store := &fakeTaskStore{}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(context.Context, database.BackgroundTask) error { return nil }})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-1", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.completed) != 1 || store.completed[0] != "task-1" {
		t.Fatalf("completed = %#v", store.completed)
	}
	if len(store.failed) != 0 {
		t.Fatalf("failed = %#v", store.failed)
	}
}

func TestRunnerSkipsCompletionForAtomicallyFinalizedHandler(t *testing.T) {
	store := &fakeTaskStore{}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(context.Context, database.BackgroundTask) error { return ErrTaskFinalizedByHandler }})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-finalized", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.completed) != 0 || len(store.failed) != 0 {
		t.Fatalf("runner wrote a second outcome: completed=%#v failed=%#v", store.completed, store.failed)
	}
}

func TestRunnerRetriesHandlerFailure(t *testing.T) {
	store := &fakeTaskStore{}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(context.Context, database.BackgroundTask) error { return errors.New("decoder unavailable") }})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-2", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.failed) != 1 || store.failed[0].code != "handler_failed" || store.failed[0].message != "decoder unavailable" {
		t.Fatalf("failed = %#v", store.failed)
	}
	wantRetry := time.Date(2026, 9, 7, 23, 0, 2, 0, time.UTC)
	if !store.failed[0].retryAt.Equal(wantRetry) {
		t.Fatalf("retry at = %v, want %v", store.failed[0].retryAt, wantRetry)
	}
}

func TestRunnerDispositionForUnknownTaskKind(t *testing.T) {
	store := &fakeTaskStore{}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(context.Context, database.BackgroundTask) error { return nil }})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-3", Kind: "metadata"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.failed) != 1 || store.failed[0].code != "unsupported_task_kind" {
		t.Fatalf("failed = %#v", store.failed)
	}
}

func TestRunnerRenewsLeaseDuringLongHandler(t *testing.T) {
	renewed := make(chan struct{})
	store := &fakeTaskStore{renewedCh: renewed}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(ctx context.Context, _ database.BackgroundTask) error {
		select {
		case <-renewed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			return errors.New("lease was not renewed")
		}
	}})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-4", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.renewed == 0 {
		t.Fatal("expected at least one lease renewal")
	}
	if len(store.completed) != 1 {
		t.Fatalf("completed = %#v", store.completed)
	}
}

func TestRunnerCancelsHandlerAndDoesNotWriteOutcomeAfterLeaseLoss(t *testing.T) {
	store := &fakeTaskStore{renewErr: database.ErrBackgroundTaskLeaseLost}
	runner := newTestRunner(t, store, map[string]Handler{"thumbnail": func(ctx context.Context, _ database.BackgroundTask) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
			return errors.New("handler was not canceled after lease loss")
		}
	}})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-5", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.renewed == 0 {
		t.Fatal("expected lease renewal attempt")
	}
	if store.recovered != 1 {
		t.Fatalf("recover calls after lease loss = %d, want 1", store.recovered)
	}
	if len(store.completed) != 0 || len(store.failed) != 0 {
		t.Fatalf("outcome written after lease loss: completed=%#v failed=%#v", store.completed, store.failed)
	}
}
