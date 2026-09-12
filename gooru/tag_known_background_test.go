package gooru

import (
	"testing"

	"gooru.local/types"
)

func TestTagKnownFilesWithBackgroundTasksRollsBackRegistrationWhenEnqueueFails(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	location := types.LocationInfo{Path: "/library/item.jpg", Hash: "hash-atomic-import", Size: 12, ModTime: 34, Extension: ".jpg"}
	_, err := client.TagKnownFilesWithBackgroundTasks([]types.LocationInfo{location}, nil, []BackgroundTaskRequest{{
		DedupeKey: "invalid-without-kind",
	}}, nil)
	if err == nil {
		t.Fatal("expected invalid background task to fail")
	}
	exists, lookupErr := client.ContentExists(location.Hash)
	if lookupErr != nil {
		t.Fatalf("ContentExists: %v", lookupErr)
	}
	if exists {
		t.Fatal("content registration committed despite background enqueue failure")
	}
}

func TestTagKnownFilesWithBackgroundTasksAndOperationStateCommitsAtomically(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "upload_import", Visible: true, ProgressTotal: 1})
	if err != nil {
		t.Fatalf("CreateBackgroundOperation: %v", err)
	}
	location := types.LocationInfo{Path: "/library/atomic.jpg", Hash: "hash-atomic-operation-state", Size: 12, ModTime: 34, Extension: ".jpg"}
	_, err = client.TagKnownFilesWithBackgroundTasksAndOperationState([]types.LocationInfo{location}, []string{"source:upload"}, nil, func(affectedCount int) (BackgroundOperationTransactionState, error) {
		return BackgroundOperationTransactionState{
			OperationID: operation.ID,
			Checkpoint:  map[string]any{"phase": "imported", "affected_count": affectedCount},
			Result:      map[string]any{"affected_count": affectedCount, "status": "ok"},
		}, nil
	}, nil)
	if err != nil {
		t.Fatalf("TagKnownFilesWithBackgroundTasksAndOperationState: %v", err)
	}
	exists, err := client.ContentExists(location.Hash)
	if err != nil || !exists {
		t.Fatalf("ContentExists = %v, %v; want true, nil", exists, err)
	}
	var checkpoint map[string]any
	found, err := client.GetBackgroundOperationCheckpoint(operation.ID, &checkpoint)
	if err != nil || !found || checkpoint["phase"] != "imported" {
		t.Fatalf("checkpoint = %#v, %v, %v", checkpoint, found, err)
	}
	var rawResult string
	if err := client.store.DB.QueryRow(`SELECT result_json FROM background_operations WHERE id = ?`, operation.ID).Scan(&rawResult); err != nil {
		t.Fatalf("read result_json: %v", err)
	}
	if rawResult == "" {
		t.Fatal("result_json was not persisted")
	}
}

func TestTagKnownFilesWithBackgroundTasksAndOperationStateRollsBackWhenStatePersistenceFails(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	location := types.LocationInfo{Path: "/library/rollback.jpg", Hash: "hash-operation-state-rollback", Size: 12, ModTime: 34, Extension: ".jpg"}
	_, err := client.TagKnownFilesWithBackgroundTasksAndOperationState([]types.LocationInfo{location}, nil, nil, func(affectedCount int) (BackgroundOperationTransactionState, error) {
		return BackgroundOperationTransactionState{OperationID: "operation-missing", Checkpoint: map[string]string{"phase": "imported"}, Result: map[string]int{"affected_count": affectedCount}}, nil
	}, nil)
	if err == nil {
		t.Fatal("expected missing operation state persistence to fail")
	}
	exists, lookupErr := client.ContentExists(location.Hash)
	if lookupErr != nil {
		t.Fatalf("ContentExists: %v", lookupErr)
	}
	if exists {
		t.Fatal("content registration committed despite operation state persistence failure")
	}
}
