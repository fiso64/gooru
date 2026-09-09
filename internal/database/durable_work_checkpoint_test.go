package database

import (
	"bytes"
	"testing"
)

func TestBackgroundOperationCheckpointPersistsWhileOperationIsActive(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-checkpoint", Kind: "upload_import"})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := []byte(`{"phase":"activated","replacement_count":1}`)
	if err := store.SetBackgroundOperationCheckpoint(op.ID, checkpoint); err != nil {
		t.Fatalf("SetBackgroundOperationCheckpoint: %v", err)
	}
	got, found, err := store.GetBackgroundOperationCheckpoint(op.ID)
	if err != nil {
		t.Fatalf("GetBackgroundOperationCheckpoint: %v", err)
	}
	if !found || !bytes.Equal(got, checkpoint) {
		t.Fatalf("checkpoint = (%q, %v), want %q", got, found, checkpoint)
	}
}

func TestBackgroundOperationCheckpointRejectsInvalidOversizedAndTerminalWrites(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-checkpoint", Kind: "upload_import"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundOperationCheckpoint(op.ID, []byte(`not-json`)); err == nil {
		t.Fatal("invalid JSON checkpoint unexpectedly accepted")
	}
	oversized := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, maxBackgroundOperationCheckpointBytes)...)
	oversized = append(oversized, '"')
	if err := store.SetBackgroundOperationCheckpoint(op.ID, oversized); err == nil {
		t.Fatal("oversized checkpoint unexpectedly accepted")
	}
	if _, err := db.Exec(`UPDATE background_operations SET status = 'completed', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, op.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundOperationCheckpoint(op.ID, []byte(`{"phase":"done"}`)); err == nil {
		t.Fatal("terminal checkpoint write unexpectedly accepted")
	}
}

func TestBackgroundOperationVisibilityCanBeRevealedAfterAdmission(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-hidden", Kind: "upload_import", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundOperationVisible(op.ID, true); err != nil {
		t.Fatalf("SetBackgroundOperationVisible: %v", err)
	}
	var visible int
	if err := db.QueryRow(`SELECT visible FROM background_operations WHERE id = ?`, op.ID).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 1 {
		t.Fatalf("visible = %d, want 1", visible)
	}
}
