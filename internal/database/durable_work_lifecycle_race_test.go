package database

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestClaimNextBackgroundTaskAllowsOnlyOneConcurrentWorker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "durable-work.db")
	bootstrap, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_journal=WAL&_busy_timeout=5000")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(bootstrap); err != nil {
		_ = bootstrap.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	if err := bootstrap.Close(); err != nil {
		t.Fatal(err)
	}

	storeA, err := NewStore(path, false)
	if err != nil {
		t.Fatalf("NewStore A: %v", err)
	}
	defer storeA.Close()
	storeB, err := NewStore(path, false)
	if err != nil {
		t.Fatalf("NewStore B: %v", err)
	}
	defer storeB.Close()

	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := storeA.EnqueueBackgroundTask(storeA.DB, NewBackgroundTask{
		ID:            "race-task",
		DedupeKey:     "race-task",
		Kind:          "thumbnail",
		ResourceClass: "image",
		CreatedAt:     now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}

	start := make(chan struct{})
	type result struct {
		task BackgroundTask
		ok   bool
		err  error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i, store := range []*Store{storeA, storeB} {
		wg.Add(1)
		go func(worker int, store *Store) {
			defer wg.Done()
			<-start
			task, ok, err := store.ClaimNextBackgroundTask("image", string(rune('a'+worker)), now, time.Minute)
			results <- result{task: task, ok: ok, err: err}
		}(i, store)
	}
	close(start)
	wg.Wait()
	close(results)

	claimed := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent claim: %v", result.err)
		}
		if result.ok {
			claimed++
			if result.task.ID != "race-task" || result.task.AttemptCount != 1 {
				t.Fatalf("unexpected claimed task: %+v", result.task)
			}
		}
	}
	if claimed != 1 {
		t.Fatalf("successful concurrent claims = %d, want 1", claimed)
	}

	var runningTasks, attempts int
	if err := storeA.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE id = 'race-task' AND status = 'running'`).Scan(&runningTasks); err != nil {
		t.Fatal(err)
	}
	if err := storeA.DB.QueryRow(`SELECT count(*) FROM background_task_attempts WHERE task_id = 'race-task'`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if runningTasks != 1 || attempts != 1 {
		t.Fatalf("post-race state = running %d attempts %d, want 1/1", runningTasks, attempts)
	}
}
