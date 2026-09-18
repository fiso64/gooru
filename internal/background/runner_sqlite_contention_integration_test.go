package background

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"gooru.local/internal/database"
)

type leaseRenewalContentionStore struct {
	*database.Store
	contentionObserved chan struct{}
	renewedAfter       chan struct{}
	sawContention      bool
}

func (s *leaseRenewalContentionStore) RenewBackgroundTaskLease(taskID, workerID string, now time.Time, leaseDuration time.Duration) (time.Time, error) {
	leaseUntil, err := s.Store.RenewBackgroundTaskLease(taskID, workerID, now, leaseDuration)
	if database.IsTransientSQLiteContention(err) {
		if !s.sawContention {
			s.sawContention = true
			close(s.contentionObserved)
		}
		return leaseUntil, err
	}
	if err == nil && s.sawContention {
		select {
		case <-s.renewedAfter:
		default:
			close(s.renewedAfter)
		}
	}
	return leaseUntil, err
}

type completionContentionStore struct {
	*database.Store
	contentionObserved chan struct{}
	sawContention      bool
}

func (s *completionContentionStore) CompleteBackgroundTask(taskID, workerID string, finishedAt time.Time) error {
	err := s.Store.CompleteBackgroundTask(taskID, workerID, finishedAt)
	if database.IsTransientSQLiteContention(err) && !s.sawContention {
		s.sawContention = true
		close(s.contentionObserved)
	}
	return err
}

func TestRunnerSurvivesRealSQLiteWriterContention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	store, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := database.RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	store.DB.SetMaxOpenConns(1)
	if _, err := store.DB.Exec(`PRAGMA busy_timeout = 1`); err != nil {
		t.Fatalf("set short busy timeout: %v", err)
	}

	now := time.Now().UTC()
	if _, _, err := store.EnqueueBackgroundTask(store.DB, database.NewBackgroundTask{
		ID:            "task-contention",
		DedupeKey:     "contention:test",
		Kind:          "thumbnail",
		ResourceClass: "image",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("enqueue task: %v", err)
	}

	writerDB, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal=WAL&_busy_timeout=1", dbPath))
	if err != nil {
		t.Fatalf("open writer DB: %v", err)
	}
	defer writerDB.Close()
	writerTx, err := writerDB.Begin()
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer writerTx.Rollback()
	if _, err := writerTx.Exec(`UPDATE background_tasks SET priority = priority WHERE id = ?`, "task-contention"); err != nil {
		t.Fatalf("acquire writer lock: %v", err)
	}

	handled := make(chan struct{})
	runner, err := NewRunner(RunnerConfig{
		Store:         store,
		ResourceClass: "image",
		WorkerID:      "worker-contention",
		Handlers: map[string]Handler{
			"thumbnail": func(context.Context, database.BackgroundTask) error {
				close(handled)
				return nil
			},
		},
		PollInterval: 2 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- runner.Run(ctx) }()

	// Let at least one claim attempt hit the held writer lock. Before the fix,
	// that SQLITE_BUSY escaped Runner.Run and terminated the serve process.
	time.Sleep(20 * time.Millisecond)
	select {
	case err := <-runDone:
		t.Fatalf("runner exited under transient writer contention: %v", err)
	default:
	}
	if err := writerTx.Commit(); err != nil {
		t.Fatalf("release writer lock: %v", err)
	}

	select {
	case <-handled:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not claim task after writer contention cleared")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("runner returned error after contention cleared: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop after cancellation")
	}
}

func TestRunnerSurvivesRealSQLiteContentionDuringLeaseRenewal(t *testing.T) {
	// The claim happens before the test opens and locks a second SQLite writer.
	// Keep that setup lease comfortably long so loaded CI cannot expire ownership
	// before runClaimed starts; the runner below still renews on the short 120ms
	// cadence that exercises transient contention and retry behavior.
	store, writerDB, claimed := setupClaimedSQLiteContentionTask(t, "task-renew-contention", 5*time.Second)

	writerTx, err := writerDB.Begin()
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer writerTx.Rollback()
	if _, err := writerTx.Exec(`UPDATE background_tasks SET priority = priority WHERE id = ?`, claimed.ID); err != nil {
		t.Fatalf("acquire writer lock: %v", err)
	}

	observedStore := &leaseRenewalContentionStore{
		Store:              store,
		contentionObserved: make(chan struct{}),
		renewedAfter:       make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	releaseDone := make(chan error, 1)
	go func() {
		select {
		case <-observedStore.contentionObserved:
			releaseDone <- writerTx.Commit()
		case <-ctx.Done():
			releaseDone <- ctx.Err()
		}
	}()

	runner, err := NewRunner(RunnerConfig{
		Store:         observedStore,
		ResourceClass: "image",
		WorkerID:      "worker-lifecycle",
		LeaseDuration: 120 * time.Millisecond,
		PollInterval:  2 * time.Millisecond,
		Handlers: map[string]Handler{
			"thumbnail": func(ctx context.Context, _ database.BackgroundTask) error {
				select {
				case <-observedStore.renewedAfter:
					return nil
				case <-ctx.Done():
					return fmt.Errorf("handler canceled before lease recovered from transient contention: %w", ctx.Err())
				}
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := runner.runClaimed(ctx, claimed); err != nil {
		t.Fatalf("run claimed task through renewal contention: %v", err)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("release writer lock: %v", err)
	}
	assertTaskStatus(t, store, claimed.ID, database.BackgroundWorkCompleted)
}

func TestRunnerSurvivesRealSQLiteContentionDuringTaskCompletion(t *testing.T) {
	// Keep the setup lease comfortably longer than this test. Loaded CI can pause
	// between the direct pre-claim and runClaimed; that delay must not turn this
	// completion-contention test into a lease-expiry recovery test.
	store, writerDB, claimed := setupClaimedSQLiteContentionTask(t, "task-complete-contention", 5*time.Second)

	writerTx, err := writerDB.Begin()
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer writerTx.Rollback()
	if _, err := writerTx.Exec(`UPDATE background_tasks SET priority = priority WHERE id = ?`, claimed.ID); err != nil {
		t.Fatalf("acquire writer lock: %v", err)
	}

	observedStore := &completionContentionStore{
		Store:              store,
		contentionObserved: make(chan struct{}),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	releaseDone := make(chan error, 1)
	go func() {
		select {
		case <-observedStore.contentionObserved:
			releaseDone <- writerTx.Commit()
		case <-ctx.Done():
			releaseDone <- ctx.Err()
		}
	}()

	runner, err := NewRunner(RunnerConfig{
		Store:         observedStore,
		ResourceClass: "image",
		WorkerID:      "worker-lifecycle",
		LeaseDuration: 500 * time.Millisecond,
		PollInterval:  2 * time.Millisecond,
		Handlers: map[string]Handler{
			"thumbnail": func(context.Context, database.BackgroundTask) error { return nil },
		},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := runner.runClaimed(ctx, claimed); err != nil {
		t.Fatalf("run claimed task through completion contention: %v", err)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("release writer lock: %v", err)
	}
	assertTaskStatus(t, store, claimed.ID, database.BackgroundWorkCompleted)
}

func setupClaimedSQLiteContentionTask(t *testing.T, taskID string, leaseDuration time.Duration) (*database.Store, *sql.DB, database.BackgroundTask) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	store, err := database.NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := database.RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	store.DB.SetMaxOpenConns(1)
	if _, err := store.DB.Exec(`PRAGMA busy_timeout = 1`); err != nil {
		t.Fatalf("set short busy timeout: %v", err)
	}

	now := time.Now().UTC()
	if _, _, err := store.EnqueueBackgroundTask(store.DB, database.NewBackgroundTask{
		ID:            taskID,
		DedupeKey:     "contention:" + taskID,
		Kind:          "thumbnail",
		ResourceClass: "image",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("enqueue task: %v", err)
	}
	claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-lifecycle", now, leaseDuration)
	if err != nil || !ok {
		t.Fatalf("claim task: task=%+v ok=%v err=%v", claimed, ok, err)
	}

	writerDB, err := sql.Open("sqlite3", fmt.Sprintf("%s?_journal=WAL&_busy_timeout=1", dbPath))
	if err != nil {
		t.Fatalf("open writer DB: %v", err)
	}
	t.Cleanup(func() { _ = writerDB.Close() })
	return store, writerDB, claimed
}

func assertTaskStatus(t *testing.T, store *database.Store, taskID string, want database.BackgroundWorkStatus) {
	t.Helper()
	var got string
	if err := store.DB.QueryRow(`SELECT status FROM background_tasks WHERE id = ?`, taskID).Scan(&got); err != nil {
		t.Fatalf("read task status: %v", err)
	}
	if database.BackgroundWorkStatus(got) != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
}
