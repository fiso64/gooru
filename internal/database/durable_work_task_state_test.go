package database

import (
	"bytes"
	"testing"
)

func newBackgroundTaskStateTestTask(t *testing.T) (*Store, string) {
	t.Helper()
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-task-state", Kind: "upload_import", ProgressTotal: 1})
	if err != nil {
		t.Fatalf("CreateBackgroundOperation: %v", err)
	}
	task, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:          "task-state",
		OperationID: op.ID,
		DedupeKey:   "task-state",
		Kind:        "upload.import",
	})
	if err != nil {
		t.Fatalf("EnqueueBackgroundTask: %v", err)
	}
	if !created {
		t.Fatal("background task unexpectedly deduplicated")
	}
	return store, task.ID
}

func TestBackgroundTaskCheckpointPersistsWhileTaskIsActive(t *testing.T) {
	store, taskID := newBackgroundTaskStateTestTask(t)
	checkpoint := []byte(`{"phase":"activated","replacement_count":1}`)
	if err := store.SetBackgroundTaskCheckpoint(taskID, checkpoint); err != nil {
		t.Fatalf("SetBackgroundTaskCheckpoint: %v", err)
	}
	got, found, err := store.GetBackgroundTaskCheckpoint(taskID)
	if err != nil {
		t.Fatalf("GetBackgroundTaskCheckpoint: %v", err)
	}
	if !found || !bytes.Equal(got, checkpoint) {
		t.Fatalf("checkpoint = (%q, %v), want %q", got, found, checkpoint)
	}
}

func TestBackgroundTaskCheckpointRejectsInvalidOversizedAndTerminalWrites(t *testing.T) {
	store, taskID := newBackgroundTaskStateTestTask(t)
	if err := store.SetBackgroundTaskCheckpoint(taskID, []byte(`not-json`)); err == nil {
		t.Fatal("invalid JSON checkpoint unexpectedly accepted")
	}
	oversized := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, maxBackgroundTaskCheckpointBytes)...)
	oversized = append(oversized, '"')
	if err := store.SetBackgroundTaskCheckpoint(taskID, oversized); err == nil {
		t.Fatal("oversized checkpoint unexpectedly accepted")
	}
	if _, err := store.DB.Exec(`UPDATE background_tasks SET status = 'canceled', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundTaskCheckpoint(taskID, []byte(`{"phase":"done"}`)); err == nil {
		t.Fatal("terminal checkpoint write unexpectedly accepted")
	}
}

func TestBackgroundTaskCheckpointRemainsReadableAfterTerminalTransition(t *testing.T) {
	store, taskID := newBackgroundTaskStateTestTask(t)
	checkpoint := []byte(`{"phase":"activated"}`)
	if err := store.SetBackgroundTaskCheckpoint(taskID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`UPDATE background_tasks SET status = 'canceled', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetBackgroundTaskCheckpoint(taskID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, checkpoint) {
		t.Fatalf("terminal checkpoint = (%q, %v), want %q", got, found, checkpoint)
	}
}

func TestBackgroundTaskResultIsDurableAndCompletionGated(t *testing.T) {
	store, taskID := newBackgroundTaskStateTestTask(t)
	result := []byte(`{"affected_count":1}`)
	if err := store.SetBackgroundTaskResult(taskID, result); err != nil {
		t.Fatalf("SetBackgroundTaskResult: %v", err)
	}
	if got, found, err := store.GetBackgroundTaskResult(taskID); err != nil || found || got != nil {
		t.Fatalf("active result = (%q, %v, %v), want hidden", got, found, err)
	}
	if _, err := store.DB.Exec(`UPDATE background_tasks SET status = 'completed', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetBackgroundTaskResult(taskID)
	if err != nil {
		t.Fatalf("GetBackgroundTaskResult: %v", err)
	}
	if !found || !bytes.Equal(got, result) {
		t.Fatalf("completed result = (%q, %v), want %q", got, found, result)
	}
}

func TestBackgroundTaskResultRejectsInvalidOversizedAndTerminalWrites(t *testing.T) {
	store, taskID := newBackgroundTaskStateTestTask(t)
	if err := store.SetBackgroundTaskResult(taskID, []byte(`not-json`)); err == nil {
		t.Fatal("invalid JSON result unexpectedly accepted")
	}
	oversized := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, maxBackgroundTaskResultBytes)...)
	oversized = append(oversized, '"')
	if err := store.SetBackgroundTaskResult(taskID, oversized); err == nil {
		t.Fatal("oversized result unexpectedly accepted")
	}
	if _, err := store.DB.Exec(`UPDATE background_tasks SET status = 'failed', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundTaskResult(taskID, []byte(`{"ok":true}`)); err == nil {
		t.Fatal("terminal result write unexpectedly accepted")
	}
}
