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
	var finishedAt int64
	if err := db.QueryRow(`SELECT finished_at FROM background_operations WHERE id = ?`, stale.ID).Scan(&finishedAt); err != nil {
		t.Fatalf("read recovered finish time: %v", err)
	}
	if finishedAt <= 0 {
		t.Fatalf("recovered finish time = %d, want unix milliseconds", finishedAt)
	}
}

func TestListUnattachedBackgroundOperationIDsDoesNotCancelRecoveryState(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	for _, operation := range []NewBackgroundOperation{
		{ID: "visible-receiving", Kind: "upload_import", Visible: true},
		{ID: "hidden-receiving", Kind: "upload_import", Visible: false},
		{ID: "attached-upload", Kind: "upload_import", Visible: true},
		{ID: "other", Kind: "thumbnail", Visible: true},
	} {
		if _, err := store.CreateBackgroundOperation(db, operation); err != nil {
			t.Fatal(err)
		}
	}
	if _, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:            "attached-task",
		OperationID:   "attached-upload",
		DedupeKey:     "attached-upload-task",
		Kind:          "upload_import",
		InputKey:      "attached-upload",
		ResourceClass: "upload_import",
	}); err != nil {
		t.Fatal(err)
	} else if !created {
		t.Fatal("attached task was not created")
	}

	ids, err := store.ListUnattachedBackgroundOperationIDs("upload_import")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "hidden-receiving" || ids[1] != "visible-receiving" {
		t.Fatalf("listed unattached operations = %q, want hidden and visible receiving", ids)
	}
	for _, id := range []string{"visible-receiving", "hidden-receiving", "attached-upload", "other"} {
		var status string
		if err := db.QueryRow(`SELECT status FROM background_operations WHERE id = ?`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "pending" {
			t.Fatalf("operation %s status after listing = %q, want pending", id, status)
		}
	}
}

func TestCancelUnattachedBackgroundOperationsIncludesVisibleReceivingWork(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "visible-receiving", Kind: "upload_import", Visible: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "other", Kind: "thumbnail", Visible: true}); err != nil {
		t.Fatal(err)
	}
	count, err := store.CancelUnattachedBackgroundOperations("upload_import")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("canceled %d operations, want 1", count)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM background_operations WHERE id = 'visible-receiving'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "canceled" {
		t.Fatalf("visible receiving status = %q, want canceled", status)
	}
}
