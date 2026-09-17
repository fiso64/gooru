package gooru

import (
	"testing"

	"gooru.local/types"
)

func TestTagKnownFilesWithBackgroundTasksAndOperationStateCommitsTaskStateAtomically(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	client.SetFileRegistrationHooks(func(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		return mediaMetadataRegistrationTasksForOperation(len(event.ContentHashes) > 0, event.OperationID)
	})
	operation, tasks, err := client.CreateBackgroundOperationWithTasks(
		BackgroundOperationRequest{Kind: "upload_import", Visible: true, ProgressTotal: 2},
		[]BackgroundTaskRequest{
			{Kind: "upload_import_segment", SubjectKind: "upload_segment", SubjectID: "0"},
			{Kind: "upload_import_segment", SubjectKind: "upload_segment", SubjectID: "1"},
		},
	)
	if err != nil {
		t.Fatalf("CreateBackgroundOperationWithTasks: %v", err)
	}
	location := types.LocationInfo{Path: "/library/task-atomic.jpg", Hash: "hash-atomic-task-state", Size: 12, ModTime: 34, Extension: ".jpg"}
	_, err = client.TagKnownFilesWithBackgroundTasksAndOperationState([]types.LocationInfo{location}, nil, nil, func(affectedCount int) (BackgroundOperationTransactionState, error) {
		return BackgroundOperationTransactionState{
			TaskID:     tasks[0].ID,
			Checkpoint: map[string]any{"phase": "imported", "affected_count": affectedCount},
			Result:     map[string]any{"affected_count": affectedCount, "status": "ok"},
		}, nil
	}, nil)
	if err != nil {
		t.Fatalf("TagKnownFilesWithBackgroundTasksAndOperationState: %v", err)
	}
	exists, err := client.ContentExists(location.Hash)
	if err != nil || !exists {
		t.Fatalf("ContentExists = %v, %v; want true, nil", exists, err)
	}
	var taskCheckpoint map[string]any
	found, err := client.GetBackgroundTaskCheckpoint(tasks[0].ID, &taskCheckpoint)
	if err != nil || !found || taskCheckpoint["phase"] != "imported" {
		t.Fatalf("task checkpoint = %#v, %v, %v", taskCheckpoint, found, err)
	}
	var operationCheckpoint map[string]any
	found, err = client.GetBackgroundOperationCheckpoint(operation.ID, &operationCheckpoint)
	if err != nil || found {
		t.Fatalf("operation checkpoint unexpectedly persisted = %#v, %v, %v", operationCheckpoint, found, err)
	}
	var rawResult string
	if err := client.store.DB.QueryRow(`SELECT result_json FROM background_tasks WHERE id = ?`, tasks[0].ID).Scan(&rawResult); err != nil {
		t.Fatalf("read task result_json: %v", err)
	}
	if rawResult == "" {
		t.Fatal("task result_json was not persisted")
	}
	var metadataTasks int
	if err := client.store.DB.QueryRow(`SELECT COUNT(*) FROM background_tasks WHERE operation_id = ? AND kind = 'media.metadata-sweep'`, operation.ID).Scan(&metadataTasks); err != nil {
		t.Fatalf("count parent-bound metadata tasks: %v", err)
	}
	if metadataTasks != 1 {
		t.Fatalf("parent-bound metadata task count = %d, want 1", metadataTasks)
	}
	var standaloneSweeps int
	if err := client.store.DB.QueryRow(`SELECT COUNT(*) FROM background_operations WHERE kind = 'media.metadata-sweep'`).Scan(&standaloneSweeps); err != nil {
		t.Fatalf("count standalone metadata operations: %v", err)
	}
	if standaloneSweeps != 0 {
		t.Fatalf("standalone metadata operation count = %d, want 0", standaloneSweeps)
	}
}

func TestTagKnownFilesWithBackgroundTasksAndOperationStateRollsBackWhenTaskStatePersistenceFails(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	location := types.LocationInfo{Path: "/library/task-rollback.jpg", Hash: "hash-task-state-rollback", Size: 12, ModTime: 34, Extension: ".jpg"}
	_, err := client.TagKnownFilesWithBackgroundTasksAndOperationState([]types.LocationInfo{location}, nil, nil, func(affectedCount int) (BackgroundOperationTransactionState, error) {
		return BackgroundOperationTransactionState{TaskID: "task-missing", Checkpoint: map[string]string{"phase": "imported"}, Result: map[string]int{"affected_count": affectedCount}}, nil
	}, nil)
	if err == nil {
		t.Fatal("expected missing task state persistence to fail")
	}
	exists, lookupErr := client.ContentExists(location.Hash)
	if lookupErr != nil {
		t.Fatalf("ContentExists: %v", lookupErr)
	}
	if exists {
		t.Fatal("content registration committed despite task state persistence failure")
	}
}
