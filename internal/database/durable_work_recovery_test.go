package database

import "testing"

func TestCancelUnattachedHiddenBackgroundOperationsOnlyReleasesReservations(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	stale, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "stale-upload", Kind: "upload_import", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	attached, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "attached-upload", Kind: "upload_import", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:            "attached-task",
		OperationID:   attached.ID,
		DedupeKey:     "attached-upload-task",
		Kind:          "upload_import",
		InputKey:      attached.ID,
		ResourceClass: "upload_import",
	}); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("attached task was not created")
	}
	visible, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "visible-upload", Kind: "upload_import", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "other-kind", Kind: "thumbnail", Visible: false})
	if err != nil {
		t.Fatal(err)
	}

	count, err := store.CancelUnattachedHiddenBackgroundOperations("upload_import")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canceled %d operations, want 1", count)
	}
	for _, tc := range []struct {
		id   string
		want string
	}{
		{stale.ID, "canceled"},
		{attached.ID, "pending"},
		{visible.ID, "pending"},
		{other.ID, "pending"},
	} {
		var status string
		if err := db.QueryRow(`SELECT status FROM background_operations WHERE id = ?`, tc.id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != tc.want {
			t.Fatalf("operation %s status = %q, want %q", tc.id, status, tc.want)
		}
	}
}
