package database

import (
	"database/sql"
	"testing"
	"time"
)

func newDurableWorkTestDB(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return &Store{}, db
}

func TestCreateBackgroundOperationAndEnqueueTask(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	createdAt := time.Date(2026, time.September, 7, 21, 0, 0, 123000000, time.FixedZone("test", 2*60*60))

	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{
		ID:            "op-upload",
		Kind:          "upload",
		Visible:       true,
		ProgressTotal: 2,
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("CreateBackgroundOperation: %v", err)
	}
	if op.Status != BackgroundWorkPending || !op.Visible || op.ProgressTotal != 2 {
		t.Fatalf("unexpected operation: %+v", op)
	}
	if !op.CreatedAt.Equal(createdAt) || op.CreatedAt.Location() != time.UTC {
		t.Fatalf("created time = %v, want UTC equivalent of %v", op.CreatedAt, createdAt)
	}

	availableAt := createdAt.Add(30 * time.Second)
	task, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:            "task-thumb-1",
		OperationID:   op.ID,
		DedupeKey:     "thumbnail:file-1:v1",
		Kind:          "thumbnail",
		SubjectKind:   "file",
		SubjectID:     "file-1",
		InputKey:      "processor-v1",
		ResourceClass: "image",
		Priority:      20,
		AvailableAt:   availableAt,
		CreatedAt:     createdAt,
		MaxAttempts:   5,
	})
	if err != nil {
		t.Fatalf("EnqueueBackgroundTask: %v", err)
	}
	if !created {
		t.Fatal("first enqueue reported existing task")
	}
	if task.ID != "task-thumb-1" || task.OperationID != op.ID || task.Status != BackgroundWorkPending {
		t.Fatalf("unexpected task: %+v", task)
	}
	if task.ResourceClass != "image" || task.Priority != 20 || task.MaxAttempts != 5 {
		t.Fatalf("unexpected scheduling metadata: %+v", task)
	}
	if !task.AvailableAt.Equal(availableAt) || task.AvailableAt.Location() != time.UTC {
		t.Fatalf("available time = %v, want %v in UTC", task.AvailableAt, availableAt)
	}
}

func TestEnqueueBackgroundTaskReturnsExistingActiveTask(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	now := time.Date(2026, time.September, 7, 22, 0, 0, 0, time.UTC)

	first, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:        "task-first",
		DedupeKey: "thumbnail:file-1:v1",
		Kind:      "thumbnail",
		CreatedAt: now,
	})
	if err != nil || !created {
		t.Fatalf("first enqueue = (%+v, %v, %v), want created", first, created, err)
	}

	existing, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:            "task-second",
		DedupeKey:     first.DedupeKey,
		Kind:          "thumbnail",
		ResourceClass: "interactive",
		Priority:      99,
		CreatedAt:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if created {
		t.Fatal("duplicate enqueue unexpectedly created a task")
	}
	if existing.ID != first.ID {
		t.Fatalf("duplicate enqueue returned %q, want existing %q", existing.ID, first.ID)
	}
	if existing.ResourceClass != "default" || existing.Priority != 0 || existing.MaxAttempts != 3 {
		t.Fatalf("returned caller replacement fields instead of persisted task: %+v", existing)
	}

	var taskCount int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE dedupe_key = ?`, first.DedupeKey).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 {
		t.Fatalf("task count = %d, want 1", taskCount)
	}
}

func TestEnqueueBackgroundTaskAllowsDedupeKeyAfterTerminalHistory(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	now := time.Date(2026, time.September, 7, 22, 0, 0, 0, time.UTC)

	first, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:        "task-v1",
		DedupeKey: "thumbnail:file-1",
		Kind:      "thumbnail",
		InputKey:  "v1",
		CreatedAt: now,
	})
	if err != nil || !created {
		t.Fatalf("first enqueue = (%+v, %v, %v), want created", first, created, err)
	}
	if _, err := db.Exec(`UPDATE background_tasks SET status = 'completed', finished_at = ? WHERE id = ?`, workTimeValue(now.Add(time.Minute)), first.ID); err != nil {
		t.Fatal(err)
	}

	second, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:        "task-v2",
		DedupeKey: first.DedupeKey,
		Kind:      "thumbnail",
		InputKey:  "v2",
		CreatedAt: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("reenqueue after terminal history: %v", err)
	}
	if !created || second.ID != "task-v2" {
		t.Fatalf("reenqueue = (%+v, %v), want newly created task-v2", second, created)
	}
}

func TestEnqueueBackgroundTaskPreservesConstraintErrors(t *testing.T) {
	store, db := newDurableWorkTestDB(t)

	_, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:          "task-orphan",
		OperationID: "missing-operation",
		DedupeKey:   "orphan",
		Kind:        "thumbnail",
	})
	if err == nil {
		t.Fatal("enqueue with missing parent unexpectedly succeeded")
	}
	if created {
		t.Fatal("enqueue with constraint failure reported created")
	}

	var taskCount int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE id = 'task-orphan'`).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 {
		t.Fatalf("constraint failure inserted %d task rows", taskCount)
	}
}
