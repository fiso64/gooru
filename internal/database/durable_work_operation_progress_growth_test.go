package database

import (
	"testing"
	"time"
)

func TestBackgroundOperationLateChildrenGrowKnownProgressTotal(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 12, 6, 5, 0, 0, time.UTC)
	const operationID = "operation-growing-total"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "dynamic", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "growing-total-1", OperationID: operationID, DedupeKey: "growing-total-1", Kind: "dynamic", ResourceClass: "io", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue first child = created %v err %v", created, err)
	}
	first, ok, err := store.ClaimNextBackgroundTask("io", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim first child = ok %v err %v", ok, err)
	}
	if err := store.CompleteBackgroundTask(first.ID, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkCompleted || op.ProgressTotal != 1 || op.ProgressCompleted != 1 {
		t.Fatalf("first terminal generation = %+v", op)
	}

	for index := 2; index <= 3; index++ {
		id := "growing-total-" + string(rune('0'+index))
		if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
			ID: id, OperationID: operationID, DedupeKey: id, Kind: "dynamic", ResourceClass: "io", CreatedAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil || !created {
			t.Fatalf("enqueue child %d = created %v err %v", index, created, err)
		}
		op = readBackgroundOperationLifecycle(t, store, operationID)
		if op.Status != BackgroundWorkPending || op.ProgressTotal != int64(index) || op.ProgressCompleted != 1 {
			t.Fatalf("operation after child %d attachment = %+v", index, op)
		}
	}

	for index := 2; index <= 3; index++ {
		claimed, ok, err := store.ClaimNextBackgroundTask("io", "worker-b", now.Add(time.Duration(index)*time.Second), time.Minute)
		if err != nil || !ok {
			t.Fatalf("claim child %d = ok %v err %v", index, ok, err)
		}
		if err := store.CompleteBackgroundTask(claimed.ID, "worker-b", now.Add(time.Duration(index)*time.Second+500*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkCompleted || op.ProgressTotal != 3 || op.ProgressCompleted != 3 || op.ProgressFailed != 0 {
		t.Fatalf("final operation = %+v", op)
	}
}

func TestBackgroundOperationLateChildrenPreserveUnknownProgressTotal(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 12, 6, 10, 0, 0, time.UTC)
	const operationID = "operation-unknown-total"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "dynamic", Visible: true, ProgressTotal: 0, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "unknown-total-1", OperationID: operationID, DedupeKey: "unknown-total-1", Kind: "dynamic", ResourceClass: "io", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue first child = created %v err %v", created, err)
	}
	first, ok, err := store.ClaimNextBackgroundTask("io", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim first child = ok %v err %v", ok, err)
	}
	if err := store.CompleteBackgroundTask(first.ID, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "unknown-total-2", OperationID: operationID, DedupeKey: "unknown-total-2", Kind: "dynamic", ResourceClass: "io", CreatedAt: now.Add(2 * time.Second),
	}); err != nil || !created {
		t.Fatalf("enqueue late child = created %v err %v", created, err)
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkPending || op.ProgressTotal != 0 || op.ProgressCompleted != 1 {
		t.Fatalf("unknown total changed during late attachment: %+v", op)
	}
}
