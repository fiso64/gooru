package background

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gooru.local/internal/database"
)

const (
	defaultLeaseDuration = 30 * time.Second
	defaultPollInterval  = 500 * time.Millisecond
	defaultRetryDelay    = time.Second
)

// Every Runner shares one durable store, and expired-lease recovery is global.
// Serialize both startup recovery and rare in-process recovery after lease loss so
// SQLite WAL transactions do not race a read snapshot upgrade to the single writer
// slot when several resource workers recover at once.
var leaseRecoveryMu sync.Mutex

// ErrTaskFinalizedByHandler indicates that the handler atomically committed its domain result and task completion.
var ErrTaskFinalizedByHandler = errors.New("background task finalized by handler transaction")

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
	subscribeWake func() (<-chan struct{}, func())
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
	SubscribeWake func() (<-chan struct{}, func())
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
		subscribeWake: cfg.SubscribeWake,
	}, nil
}

// Run recovers expired work once at startup and then continuously claims work from one
// resource class until ctx is canceled. Resource-class ownership keeps scheduling concerns
// independent from the user-visible durable operation lifecycle.
func (r *Runner) Run(ctx context.Context) error {
	var wake <-chan struct{}
	if r.subscribeWake != nil {
		var unsubscribe func()
		wake, unsubscribe = r.subscribeWake()
		if unsubscribe != nil {
			defer unsubscribe()
		}
	}

	leaseRecoveryMu.Lock()
	var recovered int
	for {
		var recoverErr error
		recovered, recoverErr = r.store.RecoverExpiredBackgroundTaskLeases(r.now())
		if recoverErr == nil {
			break
		}
		if !database.IsTransientSQLiteContention(recoverErr) {
			leaseRecoveryMu.Unlock()
			return fmt.Errorf("recover expired background work: %w", recoverErr)
		}
		r.logSQLiteContention(ctx, "startup_recovery", "")
		if err := wait(ctx, r.pollInterval); err != nil {
			leaseRecoveryMu.Unlock()
			return nil
		}
	}
	leaseRecoveryMu.Unlock()
	if recovered > 0 {
		slog.WarnContext(ctx, "recovered expired background task leases",
			"count", recovered,
			"startup_worker_id", r.workerID,
		)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		task, ok, err := r.store.ClaimNextBackgroundTask(r.resourceClass, r.workerID, r.now(), r.leaseDuration)
		if err != nil {
			if database.IsTransientSQLiteContention(err) {
				r.logSQLiteContention(ctx, "claim", "")
				if err := wait(ctx, r.pollInterval); err != nil {
					return nil
				}
				continue
			}
			return fmt.Errorf("claim background task: %w", err)
		}
		if !ok {
			if err := waitForWake(ctx, r.pollInterval, wake); err != nil {
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
		retrying, err := r.failClaimed(ctx, task, "unsupported_task_kind", "no handler registered for task kind "+task.Kind)
		if err != nil {
			if errors.Is(err, database.ErrBackgroundTaskLeaseLost) {
				return r.recoverLeaseLoss(ctx, task.ID)
			}
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("fail unsupported background task %s: %w", task.ID, err)
		}
		r.logTaskFailure(ctx, task, "unsupported_task_kind", retrying)
		return nil
	}

	handlerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopRenew := make(chan struct{})
	renewDone := make(chan error, 1)
	go r.renewLease(handlerCtx, cancel, task.ID, stopRenew, renewDone)

	err := handler(handlerCtx, task)
	handlerFinalized := errors.Is(err, ErrTaskFinalizedByHandler)
	close(stopRenew)
	renewErr := <-renewDone

	if renewErr != nil {
		if handlerFinalized && errors.Is(renewErr, database.ErrBackgroundTaskLeaseLost) {
			return nil
		}
		// Lease loss is the ownership boundary. Never write a handler outcome after it.
		if errors.Is(renewErr, database.ErrBackgroundTaskLeaseLost) {
			return r.recoverLeaseLoss(ctx, task.ID)
		}
		return fmt.Errorf("renew background task %s lease: %w", task.ID, renewErr)
	}
	if handlerFinalized {
		return nil
	}
	if ctx.Err() != nil {
		// Leave the task leased. Restart recovery will abandon/requeue it after expiry,
		// preserving the distinction between shutdown and a real handler failure.
		return nil
	}
	if err == nil {
		if err := r.completeClaimed(ctx, task); err != nil {
			if errors.Is(err, database.ErrBackgroundTaskLeaseLost) {
				return r.recoverLeaseLoss(ctx, task.ID)
			}
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("complete background task %s: %w", task.ID, err)
		}
		return nil
	}

	retrying, failErr := r.failClaimed(ctx, task, "handler_failed", err.Error())
	if failErr != nil {
		if errors.Is(failErr, database.ErrBackgroundTaskLeaseLost) {
			return r.recoverLeaseLoss(ctx, task.ID)
		}
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("fail background task %s: %w", task.ID, failErr)
	}
	r.logTaskFailure(ctx, task, "handler_failed", retrying)
	return nil
}

// recoverLeaseLoss makes runtime lease expiry self-healing. Without this path an expired
// task discovered by renewal/finalization would remain in running state until the next
// process restart, because ordinary claims only consider pending work.
func (r *Runner) recoverLeaseLoss(ctx context.Context, taskID string) error {
	if ctx.Err() != nil {
		return nil
	}
	leaseRecoveryMu.Lock()
	defer leaseRecoveryMu.Unlock()
	for {
		recovered, err := r.store.RecoverExpiredBackgroundTaskLeases(r.now())
		if err == nil {
			if recovered > 0 {
				slog.WarnContext(ctx, "recovered expired background task leases after runtime lease loss",
					"count", recovered,
					"task_id", taskID,
					"resource_class", r.resourceClass,
					"worker_id", r.workerID,
				)
			}
			return nil
		}
		if !database.IsTransientSQLiteContention(err) {
			return fmt.Errorf("recover expired background work after lease loss: %w", err)
		}
		r.logSQLiteContention(ctx, "lease_loss_recovery", taskID)
		if err := wait(ctx, r.pollInterval); err != nil {
			return nil
		}
	}
}

func (r *Runner) completeClaimed(ctx context.Context, task database.BackgroundTask) error {
	for {
		err := r.store.CompleteBackgroundTask(task.ID, r.workerID, r.now())
		if err == nil || errors.Is(err, database.ErrBackgroundTaskLeaseLost) {
			return err
		}
		if !database.IsTransientSQLiteContention(err) {
			return err
		}
		r.logSQLiteContention(ctx, "complete", task.ID)
		if err := r.renewForFinalization(ctx, task.ID); err != nil {
			return err
		}
		if err := wait(ctx, r.pollInterval); err != nil {
			return err
		}
	}
}

func (r *Runner) failClaimed(ctx context.Context, task database.BackgroundTask, errorCode, errorMessage string) (bool, error) {
	for {
		now := r.now()
		retrying, err := r.store.FailBackgroundTask(task.ID, r.workerID, now, now.Add(r.retryDelay), errorCode, errorMessage)
		if err == nil || errors.Is(err, database.ErrBackgroundTaskLeaseLost) {
			return retrying, err
		}
		if !database.IsTransientSQLiteContention(err) {
			return false, err
		}
		r.logSQLiteContention(ctx, "fail", task.ID)
		if err := r.renewForFinalization(ctx, task.ID); err != nil {
			return false, err
		}
		if err := wait(ctx, r.pollInterval); err != nil {
			return false, err
		}
	}
}

// renewForFinalization keeps ownership live while a completed handler waits for
// SQLite writer contention to clear before its durable outcome can be committed.
// If the lease expires while contention persists, lease loss wins and the stale
// worker must leave the task for normal recovery/retry.
func (r *Runner) renewForFinalization(ctx context.Context, taskID string) error {
	for {
		_, err := r.store.RenewBackgroundTaskLease(taskID, r.workerID, r.now(), r.leaseDuration)
		if err == nil || errors.Is(err, database.ErrBackgroundTaskLeaseLost) {
			return err
		}
		if !database.IsTransientSQLiteContention(err) {
			return err
		}
		r.logSQLiteContention(ctx, "finalization_renew", taskID)
		if err := wait(ctx, r.pollInterval); err != nil {
			return err
		}
	}
}

// logTaskFailure makes hidden/background task failures diagnosable without
// copying handler error strings into logs. Handler errors may contain protected
// filesystem paths; the durable database remains the detailed diagnostic source.
func (r *Runner) logTaskFailure(ctx context.Context, task database.BackgroundTask, errorCode string, retrying bool) {
	args := []any{
		"task_id", task.ID,
		"operation_id", task.OperationID,
		"kind", task.Kind,
		"resource_class", r.resourceClass,
		"worker_id", r.workerID,
		"error_code", errorCode,
	}
	if retrying {
		slog.DebugContext(ctx, "background task failed; retry scheduled", args...)
		return
	}
	slog.WarnContext(ctx, "background task failed permanently", args...)
}

func (r *Runner) logSQLiteContention(ctx context.Context, phase, taskID string) {
	args := []any{
		"phase", phase,
		"resource_class", r.resourceClass,
		"worker_id", r.workerID,
	}
	if taskID != "" {
		args = append(args, "task_id", taskID)
	}
	slog.DebugContext(ctx, "background task lifecycle delayed by sqlite contention", args...)
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
			for {
				_, err := r.store.RenewBackgroundTaskLease(taskID, r.workerID, r.now(), r.leaseDuration)
				if err == nil {
					break
				}
				if !database.IsTransientSQLiteContention(err) {
					cancel()
					done <- err
					return
				}
				r.logSQLiteContention(ctx, "renew", taskID)
				timer := time.NewTimer(r.pollInterval)
				select {
				case <-stop:
					if !timer.Stop() {
						<-timer.C
					}
					done <- nil
					return
				case <-ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					done <- nil
					return
				case <-timer.C:
				}
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

func waitForWake(ctx context.Context, duration time.Duration, wake <-chan struct{}) error {
	if wake == nil {
		return wait(ctx, duration)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case _, ok := <-wake:
		if !ok {
			return wait(ctx, duration)
		}
		return nil
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
