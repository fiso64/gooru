package gooru

import (
	"testing"
	"time"
)

func associatedBackgroundTaskRequest(producerOperationID, dedupeKey string) BackgroundTaskRequest {
	return BackgroundTaskRequest{
		OperationID: producerOperationID,
		Operation: &BackgroundOperationRequest{
			Kind:    "auxiliary-test",
			Visible: true,
		},
		OperationBinding:          BackgroundOperationAssociateWithProducer,
		CoalescePendingEquivalent: true,
		DedupeKey:                 dedupeKey,
		Kind:                      "auxiliary-test.task",
		SubjectKind:               "library",
		SubjectID:                 "association",
		InputKey:                  "wake",
		ResourceClass:             "auxiliary-test",
		MaxAttempts:               1,
	}
}

func TestAssociatedBackgroundOperationIsSeparateAndHeldUntilProducerTerminal(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	producer, err := client.CreateBackgroundOperation(BackgroundOperationRequest{
		Kind:          "producer-test",
		Visible:       true,
		ProgressTotal: 7,
	})
	if err != nil {
		t.Fatalf("create producer operation: %v", err)
	}

	first, created, err := client.EnqueueBackgroundTask(associatedBackgroundTaskRequest(producer.ID, "wake-a"))
	if err != nil {
		t.Fatalf("enqueue associated task: %v", err)
	}
	if !created {
		t.Fatal("first associated task reused existing work")
	}
	if first.OperationID == "" || first.OperationID == producer.ID {
		t.Fatalf("associated task operation = %q, want separate from producer %q", first.OperationID, producer.ID)
	}

	second, created, err := client.EnqueueBackgroundTask(associatedBackgroundTaskRequest(producer.ID, "wake-b"))
	if err != nil {
		t.Fatalf("coalesce associated task: %v", err)
	}
	if created || second.ID != first.ID || second.OperationID != first.OperationID {
		t.Fatalf("second associated task = %+v created=%v, want existing %+v", second, created, first)
	}

	var associatedOperationID string
	if err := client.store.DB.QueryRow(`
		SELECT auxiliary_operation_id
		FROM background_operation_associations
		WHERE producer_operation_id = ? AND auxiliary_kind = ?
	`, producer.ID, "auxiliary-test").Scan(&associatedOperationID); err != nil {
		t.Fatalf("read operation association: %v", err)
	}
	if associatedOperationID != first.OperationID {
		t.Fatalf("associated operation = %q, want %q", associatedOperationID, first.OperationID)
	}

	var progressTotal int64
	var producerChildren int
	if err := client.store.DB.QueryRow(`SELECT progress_total FROM background_operations WHERE id = ?`, producer.ID).Scan(&progressTotal); err != nil {
		t.Fatalf("read producer progress: %v", err)
	}
	if err := client.store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = ?`, producer.ID).Scan(&producerChildren); err != nil {
		t.Fatalf("count producer children: %v", err)
	}
	if progressTotal != 7 || producerChildren != 0 {
		t.Fatalf("producer accounting = total %d children %d, want 7 and 0", progressTotal, producerChildren)
	}

	now := time.Now().UTC()
	claimed, ok, err := client.store.ClaimNextBackgroundTask("auxiliary-test", "worker-a", now, time.Minute)
	if err != nil || !ok || claimed.ID != first.ID {
		t.Fatalf("claim associated task = (%+v, %v, %v), want %q", claimed, ok, err, first.ID)
	}
	if err := client.store.CompleteBackgroundTask(claimed.ID, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatalf("complete associated task: %v", err)
	}
	var auxiliaryStatus string
	if err := client.store.DB.QueryRow(`SELECT status FROM background_operations WHERE id = ?`, first.OperationID).Scan(&auxiliaryStatus); err != nil {
		t.Fatalf("read held auxiliary status: %v", err)
	}
	if auxiliaryStatus != "running" {
		t.Fatalf("drained auxiliary status = %q, want running while producer is active", auxiliaryStatus)
	}

	canceled, err := client.CancelBackgroundOperation(producer.ID)
	if err != nil {
		t.Fatalf("cancel producer: %v", err)
	}
	if !canceled {
		t.Fatal("producer cancellation reported no change")
	}
	if err := client.store.DB.QueryRow(`SELECT status FROM background_operations WHERE id = ?`, first.OperationID).Scan(&auxiliaryStatus); err != nil {
		t.Fatalf("read released auxiliary status: %v", err)
	}
	if auxiliaryStatus != "completed" {
		t.Fatalf("released auxiliary status = %q, want completed", auxiliaryStatus)
	}
}
