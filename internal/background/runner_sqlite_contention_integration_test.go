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
