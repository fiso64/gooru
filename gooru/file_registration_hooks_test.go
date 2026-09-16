package gooru

import (
	"path/filepath"
	"reflect"
	"testing"

	"gooru.local/types"
)

func TestFileRegistrationBackgroundTasksDeduplicatesContentHashesPerTransaction(t *testing.T) {
	client := &Client{}
	var got FileRegistrationEvent
	client.SetFileRegistrationHooks(func(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		got = event
		return []BackgroundTaskRequest{{Kind: "test.registration"}}, nil
	})

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"b", "a", "b", "", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Kind != "test.registration" {
		t.Fatalf("tasks = %#v, want one test.registration task", tasks)
	}
	if !reflect.DeepEqual(got.ContentHashes, []string{"b", "a"}) {
		t.Fatalf("content hashes = %#v, want [b a]", got.ContentHashes)
	}
}

func TestFileRegistrationHookReceivesParentOperation(t *testing.T) {
	client := &Client{}
	var got FileRegistrationEvent
	client.SetFileRegistrationHooks(func(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		got = event
		return nil, nil
	})
	if _, err := client.fileRegistrationBackgroundTasksForOperation([]string{"hash"}, " upload-op "); err != nil {
		t.Fatal(err)
	}
	if got.OperationID != "upload-op" {
		t.Fatalf("operation id = %q, want upload-op", got.OperationID)
	}
}

func TestFileRegistrationBackgroundTasksSkipsEmptyEvents(t *testing.T) {
	client := &Client{}
	called := false
	client.SetFileRegistrationHooks(func(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		called = true
		return nil, nil
	})

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"", ""})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("registration hook called for empty content identities")
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %#v, want none", tasks)
	}
}

func TestFileRegistrationHooksCanDisableAndResetDefaults(t *testing.T) {
	client := &Client{}
	client.SetFileRegistrationHooks()

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("disabled hooks produced tasks: %#v", tasks)
	}

	client.ResetFileRegistrationHooks()
	tasks, err = client.fileRegistrationBackgroundTasks([]string{"hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("reset hooks produced %d tasks, want one immediate wake: %#v", len(tasks), tasks)
	}
	if task := tasks[0]; task.Kind != BackgroundMediaMetadataSweepTaskKind || task.OperationBinding != BackgroundOperationReuseActive {
		t.Fatalf("reset hooks produced unexpected task: %#v", task)
	}
}

func registerKnownContentWithoutHooks(t *testing.T, client *Client, path, hash string) types.LocationInfo {
	t.Helper()
	file := types.LocationInfo{Path: path, Hash: hash, Size: 1, ModTime: 1}
	client.SetFileRegistrationHooks()
	if _, err := client.TagKnownFiles([]types.LocationInfo{file}, nil, nil); err != nil {
		t.Fatalf("register existing content: %v", err)
	}
	client.ResetFileRegistrationHooks()
	return file
}

func TestDefaultFileRegistrationHookSkipsKnownContent(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	registerKnownContentWithoutHooks(t, client, filepath.Join(t.TempDir(), "existing.jpg"), "existing-hash")

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"existing-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("known content produced metadata sweep tasks: %#v", tasks)
	}

	tasks, err = client.fileRegistrationBackgroundTasks([]string{"new-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Kind != BackgroundMediaMetadataSweepTaskKind {
		t.Fatalf("new content produced tasks %#v, want one metadata sweep wake", tasks)
	}
}

func TestRetagKnownFileDoesNotEnqueueMetadataSweep(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	file := registerKnownContentWithoutHooks(t, client, filepath.Join(t.TempDir(), "existing.jpg"), "existing-hash")

	if _, err := client.TagKnownFiles([]types.LocationInfo{file}, []string{"rating:safe"}, nil); err != nil {
		t.Fatalf("retag existing content: %v", err)
	}
	var count int
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE kind = ?`, BackgroundMediaMetadataSweepTaskKind).Scan(&count); err != nil {
		t.Fatalf("count metadata sweep tasks: %v", err)
	}
	if count != 0 {
		t.Fatalf("metadata sweep task count after retag = %d, want 0", count)
	}
}

func TestKnownFileRegistrationSeparatesMetadataSweepFromParentOperation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "upload.import", Visible: true, ProgressTotal: 63})
	if err != nil {
		t.Fatalf("create upload operation: %v", err)
	}
	file := types.LocationInfo{Path: filepath.Join(t.TempDir(), "new.jpg"), Hash: "new-hash", Size: 1, ModTime: 1}
	_, err = client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState([]types.LocationInfo{file}, nil, nil, func(int) (BackgroundOperationTransactionState, error) {
		return BackgroundOperationTransactionState{OperationID: operation.ID}, nil
	}, nil)
	if err != nil {
		t.Fatalf("register file in upload operation: %v", err)
	}

	var count int
	var metadataOperationID string
	if err := client.store.DB.QueryRow(`SELECT count(*), min(operation_id) FROM background_tasks WHERE kind = ?`, BackgroundMediaMetadataSweepTaskKind).Scan(&count, &metadataOperationID); err != nil {
		t.Fatalf("inspect metadata wake: %v", err)
	}
	if count != 1 || metadataOperationID == "" || metadataOperationID == operation.ID {
		t.Fatalf("metadata wakes = %d on operation %q, want one separate from upload %q", count, metadataOperationID, operation.ID)
	}
	var visible int
	if err := client.store.DB.QueryRow(`SELECT visible FROM background_operations WHERE id = ? AND kind = ?`, metadataOperationID, BackgroundMediaMetadataSweepOperationKind).Scan(&visible); err != nil {
		t.Fatalf("inspect metadata operation: %v", err)
	}
	if visible != 0 {
		t.Fatalf("upload-triggered metadata operation visible = %d, want hidden", visible)
	}
	var progressTotal int64
	var uploadChildren int
	if err := client.store.DB.QueryRow(`SELECT progress_total FROM background_operations WHERE id = ?`, operation.ID).Scan(&progressTotal); err != nil {
		t.Fatalf("inspect upload progress total: %v", err)
	}
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = ?`, operation.ID).Scan(&uploadChildren); err != nil {
		t.Fatalf("count upload children: %v", err)
	}
	if progressTotal != 63 || uploadChildren != 0 {
		t.Fatalf("upload accounting after metadata registration = total %d children %d, want 63 and 0", progressTotal, uploadChildren)
	}
}

func TestMediaMetadataRegistrationTasksUsesWakeOnlySignal(t *testing.T) {
	tasks, err := mediaMetadataRegistrationTasks(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("no-registration signal produced tasks: %#v", tasks)
	}

	tasks, err = mediaMetadataRegistrationTasks(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("registration signal produced %d tasks, want one immediate wake", len(tasks))
	}
	task := tasks[0]
	if task.Kind != BackgroundMediaMetadataSweepTaskKind || task.OperationBinding != BackgroundOperationReuseActive {
		t.Fatalf("registration signal produced unexpected task: %#v", task)
	}
	if task.Operation == nil || task.Operation.Kind != BackgroundMediaMetadataSweepOperationKind || !task.Operation.Visible {
		t.Fatalf("registration signal produced unexpected operation: %#v", task.Operation)
	}
	if !task.AvailableAt.IsZero() {
		t.Fatalf("registration wake is delayed until %v", task.AvailableAt)
	}
}

func TestMediaMetadataRegistrationTasksUseParentOperation(t *testing.T) {
	tasks, err := mediaMetadataRegistrationTasksForOperation(true, " upload-op ")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("parent-bound registration produced %d tasks, want one", len(tasks))
	}
	task := tasks[0]
	if task.OperationID != "upload-op" {
		t.Fatalf("operation id = %q, want upload-op", task.OperationID)
	}
	if task.Operation != nil {
		t.Fatalf("parent-bound registration unexpectedly creates operation: %#v", task.Operation)
	}
	if task.InputKey != mediaMetadataRegistrationInputKey || !task.CoalescePendingEquivalent {
		t.Fatalf("parent-bound registration produced unexpected wake: %#v", task)
	}
}

func TestMediaMetadataRegistrationTasksForProducerOperationUsesHiddenSweep(t *testing.T) {
	tasks, err := mediaMetadataRegistrationTasksForProducerOperation(true, " upload-op ")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("producer registration produced %d tasks, want one", len(tasks))
	}
	task := tasks[0]
	if task.OperationID != "" {
		t.Fatalf("producer registration bound metadata wake to producer %q", task.OperationID)
	}
	if task.Operation == nil || task.Operation.Kind != BackgroundMediaMetadataSweepOperationKind || task.Operation.Visible {
		t.Fatalf("producer registration produced unexpected hidden operation: %#v", task.Operation)
	}
	if task.OperationBinding != BackgroundOperationReuseActive || task.InputKey != mediaMetadataRegistrationInputKey || !task.CoalescePendingEquivalent {
		t.Fatalf("producer registration produced unexpected wake: %#v", task)
	}
}
