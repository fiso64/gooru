package gooru

import (
	"testing"
	"time"
)

func TestBackgroundOperationReadExposesLifecycleState(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, err := client.CreateBackgroundOperation(BackgroundOperationRequest{
		Kind:          "library-delete",
		Visible:       true,
		ProgressTotal: 5,
	})
	if err != nil {
		t.Fatalf("create operation: %v", err)
	}

	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	finishedAt := startedAt.Add(30 * time.Second)
	if _, err := client.store.DB.Exec(`
		UPDATE background_operations
		SET status = 'failed', progress_completed = 3, progress_failed = 2,
		    started_at = ?, finished_at = ?, error_code = 'partial_failure',
		    error_message = '2 files failed'
		WHERE id = ?
	`, startedAt.UnixMilli(), finishedAt.UnixMilli(), operation.ID); err != nil {
		t.Fatalf("update operation state: %v", err)
	}

	state, found, err := client.GetBackgroundOperation(operation.ID)
	if err != nil {
		t.Fatalf("get operation: %v", err)
	}
	if !found {
		t.Fatal("created operation not found")
	}
	if state.ID != operation.ID || state.Kind != "library-delete" || !state.Visible {
		t.Fatalf("unexpected operation identity: %+v", state)
	}
	if state.Status != BackgroundWorkFailed || state.ProgressTotal != 5 || state.ProgressCompleted != 3 || state.ProgressFailed != 2 {
		t.Fatalf("unexpected operation progress: %+v", state)
	}
	if state.StartedAt == nil || !state.StartedAt.Equal(startedAt) || state.FinishedAt == nil || !state.FinishedAt.Equal(finishedAt) {
		t.Fatalf("unexpected operation timestamps: %+v", state)
	}
	if state.ErrorCode != "partial_failure" || state.ErrorMessage != "2 files failed" {
		t.Fatalf("unexpected operation error: %+v", state)
	}

	if _, found, err := client.GetBackgroundOperation("operation-missing"); err != nil || found {
		t.Fatalf("missing operation: found=%t err=%v", found, err)
	}
}

func TestBackgroundOperationBatchRead(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	first, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "first", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "second", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET status = 'running' WHERE id = ?`, second.ID); err != nil {
		t.Fatal(err)
	}

	states, err := client.GetBackgroundOperations([]string{second.ID, "missing", first.ID})
	if err != nil {
		t.Fatalf("batch read: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("states = %+v", states)
	}
	if got := states[first.ID]; got.ID != first.ID || got.Kind != "first" || !got.Visible {
		t.Fatalf("first state = %+v", got)
	}
	if got := states[second.ID]; got.ID != second.ID || got.Kind != "second" || got.Visible || got.Status != BackgroundWorkRunning {
		t.Fatalf("second state = %+v", got)
	}
	if _, ok := states["missing"]; ok {
		t.Fatalf("missing operation unexpectedly returned: %+v", states)
	}

	empty, err := client.GetBackgroundOperations(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty batch = %+v, %v", empty, err)
	}
}

func TestBackgroundTaskReadByStableID(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, tasks, err := client.CreateBackgroundOperationWithTasks(
		BackgroundOperationRequest{Kind: "upload_import", Visible: true, ProgressTotal: 1},
		[]BackgroundTaskRequest{{Kind: "upload.import", SubjectKind: "operation", SubjectID: "logical-upload", InputKey: "segment-0"}},
	)
	if err != nil {
		t.Fatalf("create operation with task: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("created tasks = %d, want 1", len(tasks))
	}

	state, found, err := client.GetBackgroundTask(tasks[0].ID)
	if err != nil || !found {
		t.Fatalf("get task %q: found=%t err=%v", tasks[0].ID, found, err)
	}
	if state.ID != tasks[0].ID || state.OperationID != operation.ID || state.Kind != "upload.import" || state.SubjectID != "logical-upload" || state.InputKey != "segment-0" || state.Status != BackgroundWorkPending {
		t.Fatalf("unexpected task state: %+v", state)
	}
	if _, found, err := client.GetBackgroundTask("task-missing"); err != nil || found {
		t.Fatalf("missing task: found=%t err=%v", found, err)
	}
	if _, found, err := client.GetBackgroundTask(""); err != nil || found {
		t.Fatalf("empty task id: found=%t err=%v", found, err)
	}
}

func TestListBackgroundOperationsFiltersHiddenAndOrdersNewestFirst(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	visibleOld, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-old", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "hidden", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	visibleNew, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "visible-new", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET created_at = ? WHERE id = ?`, int64(100), visibleOld.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET created_at = ? WHERE id = ?`, int64(200), hidden.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET created_at = ? WHERE id = ?`, int64(300), visibleNew.ID); err != nil {
		t.Fatal(err)
	}

	visible, err := client.ListBackgroundOperations(BackgroundOperationListOptions{VisibleOnly: true})
	if err != nil {
		t.Fatalf("list visible operations: %v", err)
	}
	if len(visible) != 2 || visible[0].ID != visibleNew.ID || visible[1].ID != visibleOld.ID {
		t.Fatalf("visible operations = %+v", visible)
	}

	all, err := client.ListBackgroundOperations(BackgroundOperationListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("list all operations: %v", err)
	}
	if len(all) != 2 || all[0].ID != visibleNew.ID || all[1].ID != hidden.ID {
		t.Fatalf("all operations with limit = %+v", all)
	}
}
