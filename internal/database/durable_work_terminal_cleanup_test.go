package database

import (
	"testing"
	"time"
)

func TestFailBackgroundTaskEnqueuesTerminalCleanupOnlyAfterExhaustion(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 11, 22, 0, 0, 0, time.UTC)
	cleanup := &BackgroundTaskCleanup{
		DedupeKey:     "cleanup:retry-task",
		Kind:          "cleanup",
		SubjectKind:   "operation",
		SubjectID:     "operation-1",
		ResourceClass: "cleanup",
		Priority:      17,
		MaxAttempts:   4,
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "retry-task", DedupeKey: "retry-task", Kind: "work", ResourceClass: "work",
		MaxAttempts: 2, CreatedAt: now, TerminalFailureCleanup: cleanup,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}

	first, ok, err := store.ClaimNextBackgroundTask("work", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim = (%+v, %v, %v)", first, ok, err)
	}
	retryAt := now.Add(2 * time.Minute)
	if retry, err := store.FailBackgroundTask(first.ID, "worker-a", now.Add(time.Second), retryAt, "temporary", "retry"); err != nil || !retry {
		t.Fatalf("retryable failure = retry %v, err %v", retry, err)
	}
	assertBackgroundCleanupTaskCount(t, store, cleanup.DedupeKey, 0)

	second, ok, err := store.ClaimNextBackgroundTask("work", "worker-b", retryAt, time.Minute)
	if err != nil || !ok {
		t.Fatalf("second claim = (%+v, %v, %v)", second, ok, err)
	}
	failedAt := retryAt.Add(time.Second)
	if retry, err := store.FailBackgroundTask(second.ID, "worker-b", failedAt, time.Time{}, "permanent", "stop"); err != nil || retry {
		t.Fatalf("terminal failure = retry %v, err %v", retry, err)
	}

	var id, operationID, kind, subjectKind, subjectID, resourceClass, status string
	var priority, maxAttempts int
	var availableAt, createdAt int64
	if err := store.DB.QueryRow(`
		SELECT id, COALESCE(operation_id, ''), kind, subject_kind, subject_id, resource_class,
		       priority, status, available_at, created_at, max_attempts
		FROM background_tasks WHERE dedupe_key = ?
	`, cleanup.DedupeKey).Scan(&id, &operationID, &kind, &subjectKind, &subjectID, &resourceClass, &priority, &status, &availableAt, &createdAt, &maxAttempts); err != nil {
		t.Fatal(err)
	}
	if id != second.ID+":terminal-cleanup" || operationID != "" || kind != cleanup.Kind || subjectKind != cleanup.SubjectKind || subjectID != cleanup.SubjectID {
		t.Fatalf("cleanup identity = id %q operation %q kind %q subject %q/%q", id, operationID, kind, subjectKind, subjectID)
	}
	if resourceClass != cleanup.ResourceClass || priority != cleanup.Priority || maxAttempts != cleanup.MaxAttempts || status != "pending" {
		t.Fatalf("cleanup scheduling = resource %q priority %d attempts %d status %q", resourceClass, priority, maxAttempts, status)
	}
	if availableAt != workTimeValue(failedAt) || createdAt != workTimeValue(failedAt) {
		t.Fatalf("cleanup times = available %d created %d, want %d", availableAt, createdAt, workTimeValue(failedAt))
	}
}

func TestRecoverExpiredBackgroundTaskLeaseEnqueuesTerminalCleanup(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 11, 22, 0, 0, 0, time.UTC)
	cleanup := &BackgroundTaskCleanup{
		DedupeKey:     "cleanup:lease-task",
		Kind:          "cleanup",
		SubjectKind:   "operation",
		SubjectID:     "operation-2",
		ResourceClass: "cleanup",
		MaxAttempts:   5,
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "lease-task", DedupeKey: "lease-task", Kind: "work", ResourceClass: "work",
		MaxAttempts: 1, CreatedAt: now, TerminalFailureCleanup: cleanup,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}
	if _, ok, err := store.ClaimNextBackgroundTask("work", "worker-a", now, time.Minute); err != nil || !ok {
		t.Fatalf("claim = ok %v err %v", ok, err)
	}
	recoveredAt := now.Add(time.Minute)
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(recoveredAt); err != nil || recovered != 1 {
		t.Fatalf("recover = %d, %v", recovered, err)
	}
	assertBackgroundCleanupTaskCount(t, store, cleanup.DedupeKey, 1)

	var operationID, status string
	var availableAt int64
	if err := store.DB.QueryRow(`
		SELECT COALESCE(operation_id, ''), status, available_at
		FROM background_tasks WHERE dedupe_key = ?
	`, cleanup.DedupeKey).Scan(&operationID, &status, &availableAt); err != nil {
		t.Fatal(err)
	}
	if operationID != "" || status != "pending" || availableAt != workTimeValue(recoveredAt) {
		t.Fatalf("expired cleanup = operation %q status %q available %d", operationID, status, availableAt)
	}
}

func assertBackgroundCleanupTaskCount(t *testing.T, store *Store, dedupeKey string, want int) {
	t.Helper()
	var got int
	if err := store.DB.QueryRow(`SELECT count(*) FROM background_tasks WHERE dedupe_key = ?`, dedupeKey).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("cleanup task count for %q = %d, want %d", dedupeKey, got, want)
	}
}
