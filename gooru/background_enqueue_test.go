package gooru

import (
	"path/filepath"
	"strings"
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

func TestCreateBackgroundOperationWithTasksCommitsOwnedBatch(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, tasks, err := client.CreateBackgroundOperationWithTasks(BackgroundOperationRequest{
		Kind:          "library-delete",
		Visible:       true,
		ProgressTotal: 2,
	}, []BackgroundTaskRequest{
		{DedupeKey: "file-a:untrack", Kind: "library.delete", SubjectKind: "file", SubjectID: "file-a"},
		{Kind: "library.delete", SubjectKind: "file", SubjectID: "file-b"},
	})
	if err != nil {
		t.Fatalf("create operation with tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("task count = %d, want 2", len(tasks))
	}
	for _, task := range tasks {
		if task.OperationID != operation.ID {
			t.Fatalf("task operation = %q, want %q", task.OperationID, operation.ID)
		}
	}

	var persistedTasks int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = ?`, operation.ID).Scan(&persistedTasks); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if persistedTasks != 2 {
		t.Fatalf("persisted task count = %d, want 2", persistedTasks)
	}
	rows, err := client.store.DB.Query(`SELECT dedupe_key FROM background_tasks WHERE operation_id = ? ORDER BY subject_id`, operation.ID)
	if err != nil {
		t.Fatalf("read dedupe keys: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("scan dedupe key: %v", err)
		}
		if !strings.HasPrefix(key, operation.ID+":") {
			t.Fatalf("dedupe key %q is not scoped to operation %q", key, operation.ID)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate dedupe keys: %v", err)
	}
}

func TestCreateBackgroundOperationWithTasksRollsBackWholeBatch(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	_, _, err := client.CreateBackgroundOperationWithTasks(BackgroundOperationRequest{
		Kind:          "library-delete",
		Visible:       true,
		ProgressTotal: 2,
	}, []BackgroundTaskRequest{
		{Kind: "library.delete", SubjectKind: "file", SubjectID: "file-a"},
		{Kind: "", SubjectKind: "file", SubjectID: "file-b"},
	})
	if err == nil {
		t.Fatal("create operation with invalid task succeeded")
	}

	var operations int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_operations WHERE kind = 'library-delete'`).Scan(&operations); err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if operations != 0 {
		t.Fatalf("operation count after rollback = %d, want 0", operations)
	}
	var tasks int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE subject_id IN ('file-a', 'file-b')`).Scan(&tasks); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if tasks != 0 {
		t.Fatalf("task count after rollback = %d, want 0", tasks)
	}
}

func TestCreateBackgroundOperationWithTasksRejectsPreownedChild(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	_, _, err := client.CreateBackgroundOperationWithTasks(BackgroundOperationRequest{
		Kind:          "library-delete",
		Visible:       true,
		ProgressTotal: 1,
	}, []BackgroundTaskRequest{{
		OperationID: "operation-somewhere-else",
		Kind:        "library.delete",
		SubjectKind: "file",
		SubjectID:   "file-a",
	}})
	if err == nil {
		t.Fatal("create operation accepted a child already owned by another operation")
	}

	var operations int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_operations WHERE kind = 'library-delete'`).Scan(&operations); err != nil {
		t.Fatalf("count operations: %v", err)
	}
	if operations != 0 {
		t.Fatalf("operation count after rejected batch = %d, want 0", operations)
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
