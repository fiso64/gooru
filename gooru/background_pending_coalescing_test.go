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

func TestMediaMetadataRegistrationUsesImmediateWakeAndDebouncedLinger(t *testing.T) {
	before := time.Now().UTC()
	first, err := mediaMetadataRegistrationTasks(true)
	if err != nil {
		t.Fatalf("first registration tasks: %v", err)
	}
	second, err := mediaMetadataRegistrationTasks(true)
	if err != nil {
		t.Fatalf("second registration tasks: %v", err)
	}
	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("registration task counts = %d and %d, want 2 and 2", len(first), len(second))
	}

	for i, tasks := range [][]BackgroundTaskRequest{first, second} {
		wake, linger := tasks[0], tasks[1]
		if wake.OperationBinding != BackgroundOperationReuseActive || linger.OperationBinding != BackgroundOperationReuseActive {
			t.Fatalf("registration task set %d did not reuse the active operation", i+1)
		}
		if !wake.CoalescePendingEquivalent || !linger.CoalescePendingEquivalent {
			t.Fatalf("registration task set %d did not opt into pending-only coalescing", i+1)
		}
		if wake.PostponePendingEquivalent {
			t.Fatalf("registration task set %d immediate wake unexpectedly postpones", i+1)
		}
		if !wake.AvailableAt.IsZero() {
			t.Fatalf("registration task set %d immediate wake availability = %v, want immediate", i+1, wake.AvailableAt)
		}
		if wake.InputKey != mediaMetadataRegistrationInputKey {
			t.Fatalf("registration task set %d immediate input key = %q, want %q", i+1, wake.InputKey, mediaMetadataRegistrationInputKey)
		}
		if !linger.PostponePendingEquivalent {
			t.Fatalf("registration task set %d linger did not opt into trailing-edge postponement", i+1)
		}
		if linger.AvailableAt.Before(before.Add(mediaMetadataRegistrationDebounce)) {
			t.Fatalf("registration task set %d linger availability = %v, want at least %v after start", i+1, linger.AvailableAt, mediaMetadataRegistrationDebounce)
		}
		if linger.InputKey != mediaMetadataRegistrationLingerInputKey {
			t.Fatalf("registration task set %d linger input key = %q, want %q", i+1, linger.InputKey, mediaMetadataRegistrationLingerInputKey)
		}
		if wake.DedupeKey == linger.DedupeKey {
			t.Fatalf("registration task set %d immediate and linger dedupe keys unexpectedly match: %q", i+1, wake.DedupeKey)
		}
	}
	if first[0].DedupeKey == second[0].DedupeKey || first[1].DedupeKey == second[1].DedupeKey {
		t.Fatal("successive registration task sets unexpectedly reused generated dedupe keys")
	}
}
