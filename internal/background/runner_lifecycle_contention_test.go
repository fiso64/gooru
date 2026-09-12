package background

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"gooru.local/internal/database"
	sqlite "gosqlite.org"
)

type scriptedContentionStore struct {
	*fakeTaskStore
	mu sync.Mutex

	recoverErrors  []error
	renewErrors    []error
	completeErrors []error
	failErrors     []error

	recoverCalls  int
	renewCalls    int
	completeCalls int
	failCalls     int
}

func popScriptedError(errors []error) (error, []error, bool) {
	if len(errors) == 0 {
		return nil, errors, false
	}
	return errors[0], errors[1:], true
}

func (s *scriptedContentionStore) RecoverExpiredBackgroundTaskLeases(now time.Time) (int, error) {
	s.mu.Lock()
	s.recoverCalls++
	err, rest, scripted := popScriptedError(s.recoverErrors)
	s.recoverErrors = rest
	s.mu.Unlock()
	if scripted {
		return 0, err
	}
	return s.fakeTaskStore.RecoverExpiredBackgroundTaskLeases(now)
}

func (s *scriptedContentionStore) RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error) {
	s.mu.Lock()
	s.renewCalls++
	err, rest, scripted := popScriptedError(s.renewErrors)
	s.renewErrors = rest
	s.mu.Unlock()
	if scripted {
		return time.Time{}, err
	}
	return s.fakeTaskStore.RenewBackgroundTaskLease(taskID, workerID, now, leaseDuration)
}

func (s *scriptedContentionStore) CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error {
	s.mu.Lock()
	s.completeCalls++
	err, rest, scripted := popScriptedError(s.completeErrors)
	s.completeErrors = rest
	s.mu.Unlock()
	if scripted {
		return err
	}
	return s.fakeTaskStore.CompleteBackgroundTask(taskID, workerID, finishedAt)
}

func (s *scriptedContentionStore) FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error) {
	s.mu.Lock()
	s.failCalls++
	err, rest, scripted := popScriptedError(s.failErrors)
	s.failErrors = rest
	s.mu.Unlock()
	if scripted {
		return false, err
	}
	return s.fakeTaskStore.FailBackgroundTask(taskID, workerID, finishedAt, retryAt, errorCode, errorMessage)
}

func newLifecycleTestRunner(t *testing.T, store TaskStore, handlers map[string]Handler) *Runner {
	t.Helper()
	runner, err := NewRunner(RunnerConfig{
		Store:         store,
		ResourceClass: "image",
		WorkerID:      "worker-a",
		Handlers:      handlers,
		LeaseDuration: 30 * time.Millisecond,
		PollInterval:  time.Millisecond,
		RetryDelay:    2 * time.Second,
		Now:           func() time.Time { return time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestRunnerRetriesTransientSQLiteStartupRecoveryContention(t *testing.T) {
	store := &scriptedContentionStore{
		fakeTaskStore: &fakeTaskStore{},
		recoverErrors: []error{sqlite.ErrBusy},
	}
	runner := newLifecycleTestRunner(t, store, map[string]Handler{
		"thumbnail": func(context.Context, database.BackgroundTask) error { return nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Millisecond)
	defer cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("startup contention stopped runner: %v", err)
	}
	store.mu.Lock()
	calls := store.recoverCalls
	store.mu.Unlock()
	if calls < 2 {
		t.Fatalf("recovery calls = %d, want retry after SQLITE_BUSY", calls)
	}
}

func TestRunnerRetriesTransientSQLiteLeaseRenewalContention(t *testing.T) {
	renewed := make(chan struct{})
	store := &scriptedContentionStore{
		fakeTaskStore: &fakeTaskStore{renewedCh: renewed},
		renewErrors:   []error{sqlite.ErrLocked},
	}
	runner := newLifecycleTestRunner(t, store, map[string]Handler{
		"thumbnail": func(ctx context.Context, _ database.BackgroundTask) error {
			select {
			case <-renewed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return errors.New("lease renewal never recovered from contention")
			}
		},
	})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-renew", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	calls := store.renewCalls
	store.mu.Unlock()
	if calls < 2 {
		t.Fatalf("renew calls = %d, want retry after SQLITE_LOCKED", calls)
	}
}

func TestRunnerRetriesTransientSQLiteCompletionContentionWithLeaseRefresh(t *testing.T) {
	base := &fakeTaskStore{}
	store := &scriptedContentionStore{
		fakeTaskStore:  base,
		completeErrors: []error{sqlite.ErrBusy},
	}
	runner := newLifecycleTestRunner(t, store, map[string]Handler{
		"thumbnail": func(context.Context, database.BackgroundTask) error { return nil },
	})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-complete", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	completeCalls := store.completeCalls
	renewCalls := store.renewCalls
	store.mu.Unlock()
	if completeCalls != 2 {
		t.Fatalf("complete calls = %d, want 2", completeCalls)
	}
	if renewCalls == 0 {
		t.Fatal("completion contention did not refresh the task lease before retry")
	}
	base.mu.Lock()
	defer base.mu.Unlock()
	if len(base.completed) != 1 || base.completed[0] != "task-complete" {
		t.Fatalf("completed = %#v", base.completed)
	}
}

func TestRunnerRetriesTransientSQLiteFailureContentionWithLeaseRefresh(t *testing.T) {
	base := &fakeTaskStore{}
	store := &scriptedContentionStore{
		fakeTaskStore: base,
		failErrors:    []error{sqlite.ErrBusy},
	}
	runner := newLifecycleTestRunner(t, store, map[string]Handler{
		"thumbnail": func(context.Context, database.BackgroundTask) error { return errors.New("decoder failed") },
	})
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-fail", Kind: "thumbnail"}); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	failCalls := store.failCalls
	renewCalls := store.renewCalls
	store.mu.Unlock()
	if failCalls != 2 {
		t.Fatalf("fail calls = %d, want 2", failCalls)
	}
	if renewCalls == 0 {
		t.Fatal("failure contention did not refresh the task lease before retry")
	}
	base.mu.Lock()
	defer base.mu.Unlock()
	if len(base.failed) != 1 || base.failed[0].code != "handler_failed" {
		t.Fatalf("failed = %#v", base.failed)
	}
}

func TestRunnerStillFailsNonTransientCompletionErrors(t *testing.T) {
	store := &scriptedContentionStore{
		fakeTaskStore:  &fakeTaskStore{},
		completeErrors: []error{errors.New("disk I/O failure")},
	}
	runner := newLifecycleTestRunner(t, store, map[string]Handler{
		"thumbnail": func(context.Context, database.BackgroundTask) error { return nil },
	})
	err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-broken", Kind: "thumbnail"})
	if err == nil || !strings.Contains(err.Error(), "disk I/O failure") {
		t.Fatalf("completion error = %v, want non-transient failure", err)
	}
}
