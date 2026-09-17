package gooru

import (
	"testing"
	"time"
)

func pendingEquivalentBackgroundTaskRequest(dedupeKey string) BackgroundTaskRequest {
	return BackgroundTaskRequest{
		Operation: &BackgroundOperationRequest{
			Kind:    "pending-equivalent-test",
			Visible: true,
		},
		OperationBinding:          BackgroundOperationReuseActive,
		CoalescePendingEquivalent: true,
		DedupeKey:                 dedupeKey,
		Kind:                      "pending-equivalent-test",
		SubjectKind:               "library",
		SubjectID:                 "testing",
		InputKey:                  "registration",
		ResourceClass:             "test",
		MaxAttempts:               3,
	}
}

func concreteOperationPendingEquivalentBackgroundTaskRequest(operationID, dedupeKey string) BackgroundTaskRequest {
	request := pendingEquivalentBackgroundTaskRequest(dedupeKey)
	request.OperationID = operationID
	request.Operation = nil
	request.OperationBinding = BackgroundOperationCreateNew
	return request
}

func TestEnqueueBackgroundTaskCoalescesEquivalentPendingTask(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)

	first, created, err := client.EnqueueBackgroundTask(pendingEquivalentBackgroundTaskRequest("wake-1"))
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if !created {
		t.Fatal("first enqueue reported existing work")
	}

	second, created, err := client.EnqueueBackgroundTask(pendingEquivalentBackgroundTaskRequest("wake-2"))
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if created {
		t.Fatal("equivalent pending enqueue created a second task")
	}
	if second.ID != first.ID {
		t.Fatalf("coalesced task id = %q, want %q", second.ID, first.ID)
	}
	if second.OperationID != first.OperationID {
		t.Fatalf("coalesced operation id = %q, want %q", second.OperationID, first.OperationID)
	}

	var count int
	if err := client.store.DB.QueryRow(`
		SELECT count(*)
		FROM background_tasks
		WHERE operation_id = ? AND kind = ? AND input_key = ? AND status = 'pending'
	`, first.OperationID, first.Kind, first.InputKey).Scan(&count); err != nil {
		t.Fatalf("count pending tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("pending task count = %d, want 1", count)
	}
}

func TestEnqueueBackgroundTaskCoalescesEquivalentPendingTaskOnConcreteOperation(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	operation, err := client.CreateBackgroundOperation(BackgroundOperationRequest{
		Kind:    "concrete-parent-test",
		Visible: true,
	})
	if err != nil {
		t.Fatalf("create parent operation: %v", err)
	}

	first, created, err := client.EnqueueBackgroundTask(concreteOperationPendingEquivalentBackgroundTaskRequest(operation.ID, "wake-1"))
	if err != nil {
		t.Fatalf("first parent-bound enqueue: %v", err)
	}
	if !created {
		t.Fatal("first parent-bound enqueue reported existing work")
	}

	second, created, err := client.EnqueueBackgroundTask(concreteOperationPendingEquivalentBackgroundTaskRequest(operation.ID, "wake-2"))
	if err != nil {
		t.Fatalf("second parent-bound enqueue: %v", err)
	}
	if created {
		t.Fatal("equivalent parent-bound pending enqueue created a second task")
	}
	if second.ID != first.ID {
		t.Fatalf("coalesced parent-bound task id = %q, want %q", second.ID, first.ID)
	}
	if second.OperationID != operation.ID {
		t.Fatalf("coalesced parent-bound operation id = %q, want %q", second.OperationID, operation.ID)
	}

	var count int
	if err := client.store.DB.QueryRow(`
		SELECT count(*)
		FROM background_tasks
		WHERE operation_id = ? AND kind = ? AND input_key = ? AND status = 'pending'
	`, operation.ID, first.Kind, first.InputKey).Scan(&count); err != nil {
		t.Fatalf("count parent-bound pending tasks: %v", err)
	}
	if count != 1 {
		t.Fatalf("parent-bound pending task count = %d, want 1", count)
	}
}

func TestEnqueueBackgroundTaskCanPostponeEquivalentPendingTask(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	firstAvailableAt := time.Now().UTC().Add(time.Minute)
	firstRequest := pendingEquivalentBackgroundTaskRequest("wake-1")
	firstRequest.PostponePendingEquivalent = true
	firstRequest.AvailableAt = firstAvailableAt

	first, created, err := client.EnqueueBackgroundTask(firstRequest)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if !created {
		t.Fatal("first enqueue reported existing work")
	}

	secondAvailableAt := firstAvailableAt.Add(time.Minute)
	secondRequest := pendingEquivalentBackgroundTaskRequest("wake-2")
	secondRequest.PostponePendingEquivalent = true
	secondRequest.AvailableAt = secondAvailableAt
	second, created, err := client.EnqueueBackgroundTask(secondRequest)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if created {
		t.Fatal("equivalent pending enqueue created a second task")
	}
	if second.ID != first.ID {
		t.Fatalf("coalesced task id = %q, want %q", second.ID, first.ID)
	}

	var availableAtMillis int64
	if err := client.store.DB.QueryRow(`SELECT available_at FROM background_tasks WHERE id = ?`, first.ID).Scan(&availableAtMillis); err != nil {
		t.Fatalf("read postponed availability: %v", err)
	}
	if got, want := time.UnixMilli(availableAtMillis).UTC(), secondAvailableAt.Truncate(time.Millisecond); !got.Equal(want) {
		t.Fatalf("postponed availability = %v, want %v", got, want)
	}
}

func TestEnqueueBackgroundTaskPostponeDoesNotMovePendingTaskEarlier(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	firstAvailableAt := time.Now().UTC().Add(2 * time.Minute)
	firstRequest := pendingEquivalentBackgroundTaskRequest("wake-1")
	firstRequest.PostponePendingEquivalent = true
	firstRequest.AvailableAt = firstAvailableAt

	first, created, err := client.EnqueueBackgroundTask(firstRequest)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if !created {
		t.Fatal("first enqueue reported existing work")
	}

	secondRequest := pendingEquivalentBackgroundTaskRequest("wake-2")
	secondRequest.PostponePendingEquivalent = true
	secondRequest.AvailableAt = firstAvailableAt.Add(-time.Minute)
	second, created, err := client.EnqueueBackgroundTask(secondRequest)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if created {
		t.Fatal("equivalent pending enqueue created a second task")
	}
	if second.ID != first.ID {
		t.Fatalf("coalesced task id = %q, want %q", second.ID, first.ID)
	}

	var availableAtMillis int64
	if err := client.store.DB.QueryRow(`SELECT available_at FROM background_tasks WHERE id = ?`, first.ID).Scan(&availableAtMillis); err != nil {
		t.Fatalf("read pending availability: %v", err)
	}
	if got, want := time.UnixMilli(availableAtMillis).UTC(), firstAvailableAt.Truncate(time.Millisecond); !got.Equal(want) {
		t.Fatalf("pending availability = %v, want unchanged %v", got, want)
	}
}

func TestEnqueueBackgroundTaskCoalescingDoesNotSuppressRunningTask(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)

	first, created, err := client.EnqueueBackgroundTask(pendingEquivalentBackgroundTaskRequest("wake-1"))
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if !created {
		t.Fatal("first enqueue reported existing work")
	}
	if _, err := client.store.DB.Exec(`UPDATE background_tasks SET status = 'running' WHERE id = ?`, first.ID); err != nil {
		t.Fatalf("mark first wake running: %v", err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET status = 'running' WHERE id = ?`, first.OperationID); err != nil {
		t.Fatalf("mark operation running: %v", err)
	}

	second, created, err := client.EnqueueBackgroundTask(pendingEquivalentBackgroundTaskRequest("wake-2"))
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if !created {
		t.Fatal("running equivalent incorrectly suppressed a fresh pending wake")
	}
	if second.ID == first.ID {
		t.Fatalf("fresh pending wake reused running task id %q", first.ID)
	}
	if second.OperationID != first.OperationID {
		t.Fatalf("fresh wake operation id = %q, want %q", second.OperationID, first.OperationID)
	}

	var runningCount, pendingCount int
	if err := client.store.DB.QueryRow(`
		SELECT
			SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END)
		FROM background_tasks
		WHERE operation_id = ? AND kind = ? AND input_key = ?
	`, first.OperationID, first.Kind, first.InputKey).Scan(&runningCount, &pendingCount); err != nil {
		t.Fatalf("count running and pending tasks: %v", err)
	}
	if runningCount != 1 || pendingCount != 1 {
		t.Fatalf("task counts running=%d pending=%d, want 1 and 1", runningCount, pendingCount)
	}
}
