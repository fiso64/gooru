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

func TestCreateBackgroundOperationUsesCoreFacade(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	request := BackgroundOperationRequest{
		Kind:          "library-import",
		Visible:       true,
		ProgressTotal: 12,
	}

	operation, err := client.CreateBackgroundOperation(request)
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if operation.ID == "" || operation.Kind != request.Kind || operation.Visible != request.Visible || operation.ProgressTotal != request.ProgressTotal || operation.CreatedAt.IsZero() {
		t.Fatalf("unexpected operation: %+v", operation)
	}

	var count int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_operations WHERE id = ? AND kind = ? AND visible = 1`, operation.ID, request.Kind).Scan(&count); err != nil {
		t.Fatalf("count operation: %v", err)
	}
	if count != 1 {
		t.Fatalf("operation count = %d, want 1", count)
	}
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

func TestBackgroundOperationAndTasksCanCommitAtomicallyWithCoreMutation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	tx, err := client.store.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	operation, err := client.createBackgroundOperation(tx, BackgroundOperationRequest{
		Kind:          "library-import",
		Visible:       true,
		ProgressTotal: 1,
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("create operation in transaction: %v", err)
	}

	request := BackgroundTaskRequest{
		OperationID: operation.ID,
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

	var operationCount int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_operations WHERE id = ?`, operation.ID).Scan(&operationCount); err != nil {
		t.Fatalf("count rolled-back operation: %v", err)
	}
	if operationCount != 0 {
		t.Fatalf("rolled-back operation count = %d, want 0", operationCount)
	}

	var taskCount int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE dedupe_key = ?`, request.DedupeKey).Scan(&taskCount); err != nil {
		t.Fatalf("count rolled-back tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("rolled-back task count = %d, want 0", taskCount)
	}
}
