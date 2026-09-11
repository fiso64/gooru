package background

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gooru.local/internal/database"
)

const (
	defaultLeaseDuration = 30 * time.Second
	defaultPollInterval  = 500 * time.Millisecond
	defaultRetryDelay    = time.Second
)

// Every Runner shares one durable store in the serve process, and each Runner
// performs the same global expired-lease recovery before polling its resource
// class. Serialize that startup phase so SQLite WAL transactions do not race a
// read snapshot upgrade to the single writer slot when several resource workers
// start together after a crash.
var startupRecoveryMu sync.Mutex

type TaskStore interface {
	RecoverExpiredBackgroundTaskLeases(time.Time) (int, error)
	ClaimNextBackgroundTask(resourceClass, workerID string, now time.Time, leaseDuration time.Duration) (database.BackgroundTask, bool, error)
	RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error)
	CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error
	FailBackgroundTask(taskID, workerID string, finishedAt, retryAt time.Time, errorCode, errorMessage string) (bool, error)
}

// Handler executes one claimed task. Handlers must honor context cancellation promptly:
// the runner cancels the context when lease renewal fails so stale workers stop side effects.
type Handler func(context.Context, database.BackgroundTask) error

type Runner struct {
	store         TaskStore
	resourceClass string
	workerID      string
	handlers      map[string]Handler
	leaseDuration time.Duration
	pollInterval  time.Duration
	retryDelay    time.Duration
	now           func() time.Time
}

type RunnerConfig struct {
	Store         TaskStore
	ResourceClass string
	WorkerID      string
	Handlers      map[string]Handler
	LeaseDuration time.Duration
	PollInterval  time.Duration
	RetryDelay    time.Duration
	Now           func() time.Time
}

func NewRunner(cfg RunnerConfig) (*Runner, error) {
	if cfg.Store == nil {
		return nil, errors.New("background task store is required")
	}
	if cfg.ResourceClass == "" {
		return nil, errors.New("background task resource class is required")
	}
	if cfg.WorkerID == "" {
		return nil, errors.New("background task worker id is required")
	}
	if len(cfg.Handlers) == 0 {
		return nil, errors.New("at least one background task handler is required")
	}
	for kind, handler := range cfg.Handlers {
		if kind == "" || handler == nil {
			return nil, errors.New("background task handlers require a kind and function")
		}
	}
	if cfg.LeaseDuration <= 0 {
		cfg.LeaseDuration = defaultLeaseDuration
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.RetryDelay < 0 {
		return nil, errors.New("background task retry delay cannot be negative")
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = defaultRetryDelay
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Runner{
		store:         cfg.Store,
		resourceClass: cfg.ResourceClass,
		workerID:      cfg.WorkerID,
		handlers:      cloneHandlers(cfg.Handlers),
		leaseDuration: cfg.LeaseDuration,
		pollInterval:  cfg.PollInterval,
		retryDelay:    cfg.RetryDelay,
		now:           cfg.Now,
	}, nil
}

// Run recovers expired work once at startup and then continuously claims work from one
// resource class until ctx is canceled. Resource-class ownership keeps scheduling concerns
// independent from the user-visible durable operation lifecycle.
func (r *Runner) Run(ctx context.Context) error {
	startupRecoveryMu.Lock()
	_, recoverErr := r.store.RecoverExpiredBackgroundTaskLeases(r.now())
	startupRecoveryMu.Unlock()
	if recoverErr != nil {
		return fmt.Errorf("recover expired background work: %w", recoverErr)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		task, ok, err := r.store.ClaimNextBackgroundTask(r.resourceClass, r.workerID, r.now(), r.leaseDuration)
		if err != nil {
			return fmt.Errorf("claim background task: %w", err)
		}
		if !ok {
			if err := wait(ctx, r.pollInterval); err != nil {
				return nil
			}
			continue
		}
		if err := r.runClaimed(ctx, task); err != nil {
			return err
		}
	}
}

func (r *Runner) runClaimed(ctx context.Context, task database.BackgroundTask) error {
	handler, ok := r.handlers[task.Kind]
	if !ok {
		now := r.now()
		if _, err := r.store.FailBackgroundTask(task.ID, r.workerID, now, now.Add(r.retryDelay), "unsupported_task_kind", "no handler registered for task kind "+task.Kind); err != nil {
			return fmt.Errorf("fail unsupported background task %s: %w", task.ID, err)
		}
		return nil
	}

	handlerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopRenew := make(chan struct{})
	renewDone := make(chan error, 1)
	go r.renewLease(handlerCtx, cancel, task.ID, stopRenew, renewDone)

	err := handler(handlerCtx, task)
	close(stopRenew)
	renewErr := <-renewDone
	finishedAt := r.now()

	if renewErr != nil {
		// Lease loss is the ownership boundary. Never write a handler outcome after it.
		if errors.Is(renewErr, database.ErrBackgroundTaskLeaseLost) {
			return nil
		}
		return fmt.Errorf("renew background task %s lease: %w", task.ID, renewErr)
	}
	if ctx.Err() != nil {
		// Leave the task leased. Restart recovery will abandon/requeue it after expiry,
		// preserving the distinction between shutdown and a real handler failure.
		return nil
	}
	if err == nil {
		if err := r.store.CompleteBackgroundTask(task.ID, r.workerID, finishedAt); err != nil {
			if errors.Is(err, database.ErrBackgroundTaskLeaseLost) {
				return nil
			}
			return fmt.Errorf("complete background task %s: %w", task.ID, err)
		}
		return nil
	}

	if _, failErr := r.store.FailBackgroundTask(task.ID, r.workerID, finishedAt, finishedAt.Add(r.retryDelay), "handler_failed", err.Error()); failErr != nil {
		if errors.Is(failErr, database.ErrBackgroundTaskLeaseLost) {
			return nil
		}
		return fmt.Errorf("fail background task %s: %w", task.ID, failErr)
	}
	return nil
}

func (r *Runner) renewLease(ctx context.Context, cancel context.CancelFunc, taskID string, stop <-chan struct{}, done chan<- error) {
	interval := r.leaseDuration / 3
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			done <- nil
			return
		case <-ctx.Done():
			done <- nil
			return
		case <-ticker.C:
			if _, err := r.store.RenewBackgroundTaskLease(taskID, r.workerID, r.now(), r.leaseDuration); err != nil {
				cancel()
				done <- err
				return
			}
		}
	}
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func cloneHandlers(src map[string]Handler) map[string]Handler {
	dst := make(map[string]Handler, len(src))
	for kind, handler := range src {
		dst[kind] = handler
	}
	return dst
}
