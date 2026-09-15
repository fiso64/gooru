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

func TestMediaMetadataRegistrationUsesPendingOnlyCoalescing(t *testing.T) {
	before := time.Now().UTC()
	first, err := mediaMetadataRegistrationTasks(true)
	if err != nil {
		t.Fatalf("first registration task: %v", err)
	}
	second, err := mediaMetadataRegistrationTasks(true)
	if err != nil {
		t.Fatalf("second registration task: %v", err)
	}
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("registration task counts = %d and %d, want 1 and 1", len(first), len(second))
	}
	if !first[0].CoalescePendingEquivalent || !second[0].CoalescePendingEquivalent {
		t.Fatal("registration wake did not opt into pending-only coalescing")
	}
	if !first[0].PostponePendingEquivalent || !second[0].PostponePendingEquivalent {
		t.Fatal("registration wake did not opt into trailing-edge postponement")
	}
	if first[0].AvailableAt.Before(before.Add(mediaMetadataRegistrationDebounce)) || second[0].AvailableAt.Before(before.Add(mediaMetadataRegistrationDebounce)) {
		t.Fatalf("registration availability = %v and %v, want at least %v after start", first[0].AvailableAt, second[0].AvailableAt, mediaMetadataRegistrationDebounce)
	}
	if first[0].InputKey != mediaMetadataRegistrationInputKey || second[0].InputKey != mediaMetadataRegistrationInputKey {
		t.Fatalf("registration input keys = %q and %q, want %q", first[0].InputKey, second[0].InputKey, mediaMetadataRegistrationInputKey)
	}
	if first[0].DedupeKey == second[0].DedupeKey {
		t.Fatalf("registration wake dedupe keys unexpectedly match: %q", first[0].DedupeKey)
	}
}
