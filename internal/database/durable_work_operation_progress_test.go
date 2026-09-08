package database

import (
	"testing"
	"time"
)

func TestBackgroundOperationDoesNotFinishBeforeDeclaredProgressTotal(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 8, 2, 0, 0, 0, time.UTC)
	operationID := "operation-incremental"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "bulk-import", Visible: true, ProgressTotal: 2, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "incremental-1", OperationID: operationID, DedupeKey: "incremental-1", Kind: "thumbnail", ResourceClass: "image", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue first = created %v err %v", created, err)
	}

	first, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim first = (%+v, %v, %v)", first, ok, err)
	}
	if err := store.CompleteBackgroundTask(first.ID, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatalf("complete first: %v", err)
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkRunning || op.ProgressCompleted != 1 || op.FinishedAt != nil {
		t.Fatalf("operation finished before declared total: %+v", op)
	}

	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "incremental-2", OperationID: operationID, DedupeKey: "incremental-2", Kind: "thumbnail", ResourceClass: "image", CreatedAt: now.Add(2 * time.Second),
	}); err != nil || !created {
		t.Fatalf("enqueue second = created %v err %v", created, err)
	}
	second, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", now.Add(2*time.Second), time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim second = (%+v, %v, %v)", second, ok, err)
	}
	finished := now.Add(3 * time.Second)
	if err := store.CompleteBackgroundTask(second.ID, "worker-b", finished); err != nil {
		t.Fatalf("complete second: %v", err)
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkCompleted || op.ProgressCompleted != 2 || op.FinishedAt == nil || !op.FinishedAt.Equal(finished) {
		t.Fatalf("operation after declared total completed: %+v", op)
	}
}
