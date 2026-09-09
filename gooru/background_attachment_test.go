package gooru

import (
	"encoding/json"
	"testing"
)

func TestAttachBackgroundTaskAndRevealOperationScopesTaskAndPersistsCheckpoint(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	op, err := client.CreateBackgroundOperationWithPendingLimit(BackgroundOperationRequest{
		Kind:          "upload_import",
		Visible:       false,
		ProgressTotal: 1,
	}, 1)
	if err != nil {
		t.Fatalf("reserve operation: %v", err)
	}

	checkpoint := map[string]string{"phase": "staged"}
	task, err := client.AttachBackgroundTaskAndRevealOperation(op.ID, checkpoint, BackgroundTaskRequest{
		DedupeKey:     "import",
		Kind:          "upload.import",
		SubjectKind:   "operation",
		SubjectID:     op.ID,
		InputKey:      `{"files":[{"path":"/tmp/a"}]}`,
		ResourceClass: "upload",
		MaxAttempts:   5,
	})
	if err != nil {
		t.Fatalf("attach operation task: %v", err)
	}
	if task.OperationID != op.ID {
		t.Fatalf("task operation id = %q, want %q", task.OperationID, op.ID)
	}

	var dedupe string
	if err := client.store.DB.QueryRow(`SELECT dedupe_key FROM background_tasks WHERE id = ?`, task.ID).Scan(&dedupe); err != nil {
		t.Fatal(err)
	}
	if want := op.ID + ":import"; dedupe != want {
		t.Fatalf("task dedupe = %q, want %q", dedupe, want)
	}
	state, found, err := client.GetBackgroundOperation(op.ID)
	if err != nil || !found {
		t.Fatalf("read operation: found=%v err=%v", found, err)
	}
	if !state.Visible {
		t.Fatal("attached operation remained hidden")
	}
	var got map[string]string
	found, err = client.GetBackgroundOperationCheckpoint(op.ID, &got)
	if err != nil || !found {
		t.Fatalf("read checkpoint: found=%v err=%v", found, err)
	}
	if encoded, _ := json.Marshal(got); string(encoded) != `{"phase":"staged"}` {
		t.Fatalf("checkpoint = %s", encoded)
	}
}

func TestAttachBackgroundTaskAndRevealOperationRejectsMismatchedTaskOwner(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	op, err := client.CreateBackgroundOperationWithPendingLimit(BackgroundOperationRequest{Kind: "upload_import"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.AttachBackgroundTaskAndRevealOperation(op.ID, map[string]string{"phase": "staged"}, BackgroundTaskRequest{
		OperationID: "different-operation",
		Kind:        "upload.import",
	}); err == nil {
		t.Fatal("mismatched task owner unexpectedly accepted")
	}
	state, found, err := client.GetBackgroundOperation(op.ID)
	if err != nil || !found {
		t.Fatalf("read operation: found=%v err=%v", found, err)
	}
	if state.Visible {
		t.Fatal("rejected operation attachment changed visibility")
	}
}
