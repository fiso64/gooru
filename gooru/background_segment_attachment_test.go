package gooru

import (
	"encoding/json"
	"testing"
)

func TestAttachBackgroundTaskToOperationScopesStableChildAndCheckpoint(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	op, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "upload_import", Visible: true, ProgressTotal: 2})
	if err != nil {
		t.Fatal(err)
	}
	request := BackgroundTaskRequest{
		DedupeKey:     "import",
		Kind:          "upload.import",
		SubjectKind:   "operation",
		SubjectID:     op.ID,
		InputKey:      `{"segment":0}`,
		ResourceClass: "upload",
		MaxAttempts:   5,
	}
	task, created, err := client.AttachBackgroundTaskToOperation(op.ID, "stable-segment-0", map[string]any{"phase": "staged", "segment": 0}, request)
	if err != nil {
		t.Fatalf("attach stable child: %v", err)
	}
	if !created || task.ID != "stable-segment-0" || task.OperationID != op.ID {
		t.Fatalf("stable child = %#v created=%v", task, created)
	}
	var dedupe string
	if err := client.store.DB.QueryRow(`SELECT dedupe_key FROM background_tasks WHERE id = ?`, task.ID).Scan(&dedupe); err != nil {
		t.Fatal(err)
	}
	if want := op.ID + ":task:stable-segment-0"; dedupe != want {
		t.Fatalf("task dedupe = %q, want %q", dedupe, want)
	}
	var checkpoint map[string]any
	found, err := client.GetBackgroundTaskCheckpoint(task.ID, &checkpoint)
	if err != nil || !found {
		t.Fatalf("read task checkpoint: found=%v err=%v", found, err)
	}
	encoded, _ := json.Marshal(checkpoint)
	if string(encoded) != `{"phase":"staged","segment":0}` {
		t.Fatalf("task checkpoint = %s", encoded)
	}

	retry, created, err := client.AttachBackgroundTaskToOperation(op.ID, "stable-segment-0", map[string]string{"phase": "older-retry"}, request)
	if err != nil {
		t.Fatalf("retry stable child: %v", err)
	}
	if created || retry.ID != task.ID {
		t.Fatalf("stable retry = %#v created=%v", retry, created)
	}
	checkpoint = nil
	found, err = client.GetBackgroundTaskCheckpoint(task.ID, &checkpoint)
	if err != nil || !found {
		t.Fatalf("reread task checkpoint: found=%v err=%v", found, err)
	}
	encoded, _ = json.Marshal(checkpoint)
	if string(encoded) != `{"phase":"staged","segment":0}` {
		t.Fatalf("idempotent retry rewrote checkpoint: %s", encoded)
	}

	siblingRequest := request
	siblingRequest.InputKey = `{"segment":1}`
	sibling, created, err := client.AttachBackgroundTaskToOperation(op.ID, "stable-segment-1", map[string]any{"phase": "staged", "segment": 1}, siblingRequest)
	if err != nil {
		t.Fatalf("attach sibling with shared logical dedupe key: %v", err)
	}
	if !created || sibling.ID != "stable-segment-1" || sibling.OperationID != op.ID {
		t.Fatalf("stable sibling = %#v created=%v", sibling, created)
	}
	if err := client.store.DB.QueryRow(`SELECT dedupe_key FROM background_tasks WHERE id = ?`, sibling.ID).Scan(&dedupe); err != nil {
		t.Fatal(err)
	}
	if want := op.ID + ":task:stable-segment-1"; dedupe != want {
		t.Fatalf("sibling dedupe = %q, want %q", dedupe, want)
	}
}

func TestAttachBackgroundTaskToOperationRejectsMismatchedOwner(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	op, err := client.CreateBackgroundOperation(BackgroundOperationRequest{Kind: "upload_import", Visible: true, ProgressTotal: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.AttachBackgroundTaskToOperation(op.ID, "stable-segment-0", map[string]string{"phase": "staged"}, BackgroundTaskRequest{
		OperationID: "different-operation",
		Kind:        "upload.import",
	}); err == nil {
		t.Fatal("mismatched stable child owner unexpectedly accepted")
	}
}
