package gooru

import (
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func newBackgroundEnqueueTestClient(t *testing.T) *Client {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.store.Close() })
	return client
}

func TestEnqueueBackgroundTaskUsesDurableActiveDedupe(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	request := BackgroundTaskRequest{
		DedupeKey:     "thumbnail:content-1:grid-v1",
		Kind:          "thumbnail",
		SubjectKind:   "content",
		SubjectID:     "content-1",
		InputKey:      "grid-v1",
		ResourceClass: "image",
		Priority:      20,
		MaxAttempts:   5,
	}

	first, created, err := client.EnqueueBackgroundTask(request)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if !created {
		t.Fatal("first enqueue reported existing work")
	}
	if first.ID == "" || first.Kind != request.Kind || first.SubjectID != request.SubjectID || first.InputKey != request.InputKey {
		t.Fatalf("unexpected first task: %+v", first)
	}

	second, created, err := client.EnqueueBackgroundTask(request)
	if err != nil {
		t.Fatalf("duplicate enqueue: %v", err)
	}
	if created {
		t.Fatal("duplicate enqueue created a second active task")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate enqueue returned %q, want existing %q", second.ID, first.ID)
	}

	var count int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE dedupe_key = ?`, request.DedupeKey).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("task count = %d, want 1", count)
	}
}

func TestEnqueueBackgroundTaskCanCommitAtomicallyWithCoreMutation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	tx, err := client.store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	request := BackgroundTaskRequest{
		DedupeKey:   "thumbnail:content-rollback:grid-v1",
		Kind:        "thumbnail",
		SubjectKind: "content",
		SubjectID:   "content-rollback",
		InputKey:    "grid-v1",
	}
	if _, created, err := client.enqueueBackgroundTask(tx, request); err != nil {
		_ = tx.Rollback()
		t.Fatalf("enqueue in transaction: %v", err)
	} else if !created {
		_ = tx.Rollback()
		t.Fatal("transactional enqueue reported existing work")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	var count int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE dedupe_key = ?`, request.DedupeKey).Scan(&count); err != nil {
		t.Fatalf("count rolled-back tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back task count = %d, want 0", count)
	}
}
