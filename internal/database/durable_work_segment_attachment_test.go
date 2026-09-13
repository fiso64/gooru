package database

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestAttachBackgroundTaskToOperationAddsDeclaredChildrenWithTaskCheckpoints(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true, ProgressTotal: 2}); err != nil {
		t.Fatal(err)
	}

	for index, checkpoint := range [][]byte{[]byte(`{"phase":"staged","segment":0}`), []byte(`{"phase":"staged","segment":1}`)} {
		id := []string{"upload-task-0", "upload-task-1"}[index]
		task, created, err := store.AttachBackgroundTaskToOperation("upload-op", checkpoint, NewBackgroundTask{
			ID:            id,
			DedupeKey:     "upload-op:segment:" + string(rune('0'+index)),
			Kind:          "upload.import",
			SubjectKind:   "operation",
			SubjectID:     "upload-op",
			InputKey:      `{"segment":` + string(rune('0'+index)) + `}`,
			ResourceClass: "upload",
			MaxAttempts:   5,
		})
		if err != nil {
			t.Fatalf("attach segment %d: %v", index, err)
		}
		if !created || task.ID != id {
			t.Fatalf("attach segment %d = (%q, %v), want (%q, true)", index, task.ID, created, id)
		}
		got, found, err := store.GetBackgroundTaskCheckpoint(id)
		if err != nil || !found || !bytes.Equal(got, checkpoint) {
			t.Fatalf("task checkpoint %d = (%q, %v, %v), want %q", index, got, found, err, checkpoint)
		}
	}

	if _, found, err := store.GetBackgroundOperationCheckpoint("upload-op"); err != nil || found {
		t.Fatalf("shared operation checkpoint = found %v err %v, want absent", found, err)
	}
	if _, _, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), NewBackgroundTask{
		ID: "upload-task-2", DedupeKey: "upload-op:segment:2", Kind: "upload.import", ResourceClass: "upload",
	}); err == nil || !strings.Contains(err.Error(), "declared child count") {
		t.Fatalf("third child error = %v, want declared child count rejection", err)
	}
}

func TestAttachBackgroundTaskToOperationRetryIsIdempotentAfterTerminalState(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true, ProgressTotal: 1}); err != nil {
		t.Fatal(err)
	}
	request := NewBackgroundTask{
		ID:            "upload-task-0",
		DedupeKey:     "upload-op:segment:0",
		Kind:          "upload.import",
		SubjectKind:   "operation",
		SubjectID:     "upload-op",
		InputKey:      `{"segment":0}`,
		ResourceClass: "upload",
		MaxAttempts:   5,
	}
	if _, created, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), request); err != nil || !created {
		t.Fatalf("initial attach = created %v err %v", created, err)
	}
	advanced := []byte(`{"phase":"imported"}`)
	if err := store.SetBackgroundTaskCheckpoint(request.ID, advanced); err != nil {
		t.Fatalf("advance task checkpoint: %v", err)
	}

	now := time.Now().UTC()
	claimed, found, err := store.ClaimNextBackgroundTask("upload", "worker", now, time.Minute)
	if err != nil || !found || claimed.ID != request.ID {
		t.Fatalf("claim task = (%q, %v, %v)", claimed.ID, found, err)
	}
	if err := store.CompleteBackgroundTask(request.ID, "worker", now.Add(time.Second)); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	retried, created, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), request)
	if err != nil {
		t.Fatalf("retry terminal attachment: %v", err)
	}
	if created || retried.ID != request.ID || retried.Status != BackgroundWorkCompleted {
		t.Fatalf("terminal retry = (%q, %q, created=%v), want completed existing task", retried.ID, retried.Status, created)
	}
	got, found, err := store.GetBackgroundTaskCheckpoint(request.ID)
	if err != nil || !found || !bytes.Equal(got, advanced) {
		t.Fatalf("checkpoint after retry = (%q, %v, %v), want advanced %q", got, found, err, advanced)
	}
}

func TestAttachBackgroundTaskToOperationRejectsChangedStableTask(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true, ProgressTotal: 2}); err != nil {
		t.Fatal(err)
	}
	request := NewBackgroundTask{ID: "stable-segment", DedupeKey: "upload-op:segment:0", Kind: "upload.import", InputKey: `{"segment":0}`, ResourceClass: "upload"}
	if _, created, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), request); err != nil || !created {
		t.Fatalf("initial attach = created %v err %v", created, err)
	}
	request.InputKey = `{"segment":0,"different":true}`
	if _, _, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), request); err == nil || !strings.Contains(err.Error(), "different immutable input") {
		t.Fatalf("changed stable task error = %v, want immutable-input rejection", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = 'upload-op'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("task count after changed retry = %d, want 1", count)
	}
}

func TestAttachBackgroundTaskToOperationRejectsNewChildAfterOperationTerminal(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true, ProgressTotal: 1}); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), NewBackgroundTask{
		ID: "upload-task-0", DedupeKey: "upload-op:segment:0", Kind: "upload.import", ResourceClass: "upload",
	}); err != nil || !created {
		t.Fatalf("initial attach = created %v err %v", created, err)
	}
	now := time.Now().UTC()
	if _, found, err := store.ClaimNextBackgroundTask("upload", "worker", now, time.Minute); err != nil || !found {
		t.Fatalf("claim task = found %v err %v", found, err)
	}
	if err := store.CompleteBackgroundTask("upload-task-0", "worker", now.Add(time.Second)); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if _, _, err := store.AttachBackgroundTaskToOperation("upload-op", []byte(`{"phase":"staged"}`), NewBackgroundTask{
		ID: "upload-task-1", DedupeKey: "upload-op:segment:1", Kind: "upload.import", ResourceClass: "upload",
	}); err == nil || !strings.Contains(err.Error(), "no longer accepts") {
		t.Fatalf("new terminal child error = %v, want terminal rejection", err)
	}
}
