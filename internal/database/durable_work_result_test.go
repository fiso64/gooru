package database

import (
	"bytes"
	"testing"
)

func TestBackgroundOperationResultIsDurableAndCompletionGated(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{
		ID:      "op-result",
		Kind:    "upload_import",
		Visible: true,
	})
	if err != nil {
		t.Fatalf("CreateBackgroundOperation: %v", err)
	}
	result := []byte(`{"affected_count":1,"files":[{"name":"example.jpg","status":"imported"}]}`)
	if err := store.SetBackgroundOperationResult(op.ID, result); err != nil {
		t.Fatalf("SetBackgroundOperationResult: %v", err)
	}
	if got, found, err := store.GetBackgroundOperationResult(op.ID); err != nil || found || got != nil {
		t.Fatalf("active result = (%q, %v, %v), want hidden", got, found, err)
	}
	if _, err := db.Exec(`UPDATE background_operations SET status = 'completed', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, op.ID); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetBackgroundOperationResult(op.ID)
	if err != nil {
		t.Fatalf("GetBackgroundOperationResult: %v", err)
	}
	if !found || !bytes.Equal(got, result) {
		t.Fatalf("completed result = (%q, %v), want %q", got, found, result)
	}
}

func TestBackgroundOperationResultRejectsInvalidOrTerminalWrites(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-result", Kind: "upload_import"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundOperationResult(op.ID, []byte(`not-json`)); err == nil {
		t.Fatal("invalid JSON result unexpectedly accepted")
	}
	if _, err := db.Exec(`UPDATE background_operations SET status = 'completed', finished_at = CURRENT_TIMESTAMP WHERE id = ?`, op.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundOperationResult(op.ID, []byte(`{"ok":true}`)); err == nil {
		t.Fatal("terminal operation result write unexpectedly accepted")
	}
}

func TestBackgroundOperationResultIsBounded(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-result", Kind: "upload_import"})
	if err != nil {
		t.Fatal(err)
	}
	result := append([]byte{'"'}, bytes.Repeat([]byte{'x'}, maxBackgroundOperationResultBytes)...)
	result = append(result, '"')
	if err := store.SetBackgroundOperationResult(op.ID, result); err == nil {
		t.Fatal("oversized result unexpectedly accepted")
	}
}
