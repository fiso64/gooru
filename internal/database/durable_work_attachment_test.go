package database

import (
	"bytes"
	"testing"
)

func TestAttachBackgroundTaskAndRevealOperationCommitsAsOneUnit(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: false, ProgressTotal: 1})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := []byte(`{"phase":"staged"}`)
	task, err := store.AttachBackgroundTaskAndRevealOperation(op.ID, checkpoint, NewBackgroundTask{
		ID:            "upload-task",
		DedupeKey:     "upload-op:import",
		Kind:          "upload.import",
		InputKey:      `{"files":[{"path":"/tmp/a"}]}`,
		ResourceClass: "upload",
		MaxAttempts:   5,
	})
	if err != nil {
		t.Fatalf("AttachBackgroundTaskAndRevealOperation: %v", err)
	}
	if task.OperationID != op.ID {
		t.Fatalf("task operation id = %q, want %q", task.OperationID, op.ID)
	}

	var visible int
	if err := db.QueryRow(`SELECT visible FROM background_operations WHERE id = ?`, op.ID).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 1 {
		t.Fatalf("visible = %d, want 1", visible)
	}
	gotCheckpoint, found, err := store.GetBackgroundOperationCheckpoint(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(gotCheckpoint, checkpoint) {
		t.Fatalf("checkpoint = (%q, %v), want %q", gotCheckpoint, found, checkpoint)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = ?`, op.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("attached tasks = %d, want 1", count)
	}
}

func TestAttachBackgroundTaskAndRevealOperationRollsBackFailedTask(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AttachBackgroundTaskAndRevealOperation(op.ID, []byte(`{"phase":"staged"}`), NewBackgroundTask{
		ID:        "upload-task",
		DedupeKey: "upload-op:import",
		// Missing Kind deliberately fails after the checkpoint write inside the transaction.
	})
	if err == nil {
		t.Fatal("invalid task attachment unexpectedly succeeded")
	}

	var visible int
	var checkpoint string
	if err := db.QueryRow(`SELECT visible, COALESCE(checkpoint_json, '') FROM background_operations WHERE id = ?`, op.ID).Scan(&visible, &checkpoint); err != nil {
		t.Fatal(err)
	}
	if visible != 0 || checkpoint != "" {
		t.Fatalf("failed attachment persisted visible=%d checkpoint=%q", visible, checkpoint)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = ?`, op.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed attachment persisted %d tasks", count)
	}
}

func TestAttachBackgroundTaskAndRevealOperationRejectsAlreadyAttachedReservation(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{ID: "existing", OperationID: op.ID, DedupeKey: "existing", Kind: "upload.import"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachBackgroundTaskAndRevealOperation(op.ID, []byte(`{"phase":"staged"}`), NewBackgroundTask{ID: "second", DedupeKey: "second", Kind: "upload.import"}); err == nil {
		t.Fatal("already attached reservation unexpectedly accepted a second first task")
	}
}
