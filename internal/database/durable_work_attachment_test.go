package database

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
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

func TestAttachBackgroundTaskAndRevealOperationWaitsForConcurrentWriter(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "gooru.db"), false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	store.DB.SetMaxOpenConns(4)
	if err := RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	op, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: false})
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := store.DB.Begin()
	if err != nil {
		t.Fatalf("begin blocking writer: %v", err)
	}
	defer blocker.Rollback()
	if _, err := blocker.Exec(`UPDATE background_operations SET progress_total = progress_total WHERE id = ?`, op.ID); err != nil {
		t.Fatalf("acquire blocking writer: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := store.AttachBackgroundTaskAndRevealOperation(op.ID, []byte(`{"phase":"staged"}`), NewBackgroundTask{
			ID: "upload-task", DedupeKey: "upload-op:import", Kind: "upload.import", ResourceClass: "upload",
		})
		result <- err
	}()

	// The attachment's first statement must wait as a writer. A read-first
	// transaction would take a WAL snapshot here and fail its later upgrade with
	// SQLITE_BUSY instead of benefiting from the configured busy timeout.
	time.Sleep(100 * time.Millisecond)
	if err := blocker.Commit(); err != nil {
		t.Fatalf("commit blocking writer: %v", err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("attachment failed instead of waiting for writer: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("attachment did not complete after concurrent writer released")
	}
}

func TestAttachBackgroundTaskAcceptsVisibleReceivingReservation(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "visible-upload", Kind: "upload_import", Visible: true, ProgressTotal: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AttachBackgroundTaskAndRevealOperation("visible-upload", []byte(`{"phase":"staged"}`), NewBackgroundTask{ID: "upload-task", DedupeKey: "upload", Kind: "upload.import", SubjectKind: "operation", SubjectID: "visible-upload", ResourceClass: "upload", MaxAttempts: 5}); err != nil {
		t.Fatalf("attach visible upload: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = 'visible-upload'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("attached task count = %d, want 1", count)
	}
}


func TestBackgroundTagMutationSetupLeavesNoPartialStateOnTaskError(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db

	_, created, err := store.CreateBackgroundTagMutationWithTask(
		NewBackgroundOperation{ID: "tag-op", Kind: "tag_mutation", Visible: false, ProgressTotal: 1},
		8,
		"add",
		[]byte(`{"file_ids":["file-1"]}`),
		[]byte(`["reviewed"]`),
		BackgroundTagTargetFileID,
		[]string{"file-1"},
		"",
		nil,
		[]byte(`{"version":1}`),
		NewBackgroundTask{
			ID:            "tag-task",
			OperationID:   "tag-op",
			DedupeKey:     "tag-op:mutate",
			SubjectKind:   "operation",
			SubjectID:     "tag-op",
			InputKey:      "v1",
			ResourceClass: "metadata",
			MaxAttempts:   5,
		},
	)
	if err == nil || created {
		t.Fatalf("invalid tag task setup = created %v err %v, want an error", created, err)
	}

	var operations int
	if err := db.QueryRow("SELECT count(*) FROM background_operations").Scan(&operations); err != nil {
		t.Fatal(err)
	}
	var mutations int
	if err := db.QueryRow("SELECT count(*) FROM background_tag_mutations").Scan(&mutations); err != nil {
		t.Fatal(err)
	}
	var targets int
	if err := db.QueryRow("SELECT count(*) FROM background_tag_mutation_targets").Scan(&targets); err != nil {
		t.Fatal(err)
	}
	var tasks int
	if err := db.QueryRow("SELECT count(*) FROM background_tasks").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || mutations != 0 || targets != 0 || tasks != 0 {
		t.Fatalf("partial tag setup remained: operations=%d mutations=%d targets=%d tasks=%d", operations, mutations, targets, tasks)
	}
}
