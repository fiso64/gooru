package database

import (
	"testing"
	"time"
)

func TestBackgroundOperationWaitsForDeclaredFutureChild(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 11, 13, 20, 0, 0, time.UTC)
	const operationID = "operation-future-child"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "bulk-import", Visible: true, ProgressTotal: 2, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "future-child-1", OperationID: operationID, DedupeKey: "future-child-1", Kind: "import", ResourceClass: "io", CreatedAt: now,
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
	if op.Status != BackgroundWorkRunning || op.ProgressCompleted != 1 || op.FinishedAt != nil {
		t.Fatalf("operation before future child attachment = %+v", op)
	}

	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "future-child-2", OperationID: operationID, DedupeKey: "future-child-2", Kind: "import", ResourceClass: "io", CreatedAt: now.Add(2 * time.Second),
	}); err != nil || !created {
		t.Fatalf("enqueue future child = created %v err %v", created, err)
	}
	second, ok, err := store.ClaimNextBackgroundTask("io", "worker-b", now.Add(2*time.Second), time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim future child = ok %v err %v", ok, err)
	}
	finishedAt := now.Add(3 * time.Second)
	if err := store.CompleteBackgroundTask(second.ID, "worker-b", finishedAt); err != nil {
		t.Fatal(err)
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkCompleted || op.ProgressCompleted != 2 || op.FinishedAt == nil || !op.FinishedAt.Equal(finishedAt) {
		t.Fatalf("operation after future child = %+v", op)
	}
}

func TestBackgroundOperationLateChildReopensTerminalHistory(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 11, 13, 30, 0, 0, time.UTC)
	const operationID = "operation-late-child"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "dynamic", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 2; index++ {
		id := "late-child-" + string(rune('0'+index))
		if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
			ID: id, OperationID: operationID, DedupeKey: id, Kind: "dynamic", ResourceClass: "io", CreatedAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil || !created {
			t.Fatalf("enqueue child %d = created %v err %v", index, created, err)
		}
		claimed, ok, err := store.ClaimNextBackgroundTask("io", "worker", now.Add(time.Duration(index)*time.Second), time.Minute)
		if err != nil || !ok {
			t.Fatalf("claim child %d = ok %v err %v", index, ok, err)
		}
		if index == 1 {
			if err := store.SetBackgroundOperationResult(operationID, []byte(`{"generation":1}`)); err != nil {
				t.Fatalf("SetBackgroundOperationResult: %v", err)
			}
		}
		if index == 2 {
			op := readBackgroundOperationLifecycle(t, store, operationID)
			if op.Status != BackgroundWorkRunning || op.FinishedAt != nil || op.ErrorCode != "" {
				t.Fatalf("operation did not reopen for late child: %+v", op)
			}
			if got, found, err := store.GetBackgroundOperationResult(operationID); err != nil || found || got != nil {
				t.Fatalf("reopened operation result = (%q, %v, %v), want hidden and invalidated", got, found, err)
			}
		}
		if err := store.CompleteBackgroundTask(claimed.ID, "worker", now.Add(time.Duration(index)*time.Second+500*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		if index == 1 {
			got, found, err := store.GetBackgroundOperationResult(operationID)
			if err != nil || !found || string(got) != `{"generation":1}` {
				t.Fatalf("first completed result = (%q, %v, %v), want generation 1", got, found, err)
			}
		}
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkCompleted || op.ProgressCompleted != 2 || op.ProgressFailed != 0 {
		t.Fatalf("operation after late child history = %+v", op)
	}
	if got, found, err := store.GetBackgroundOperationResult(operationID); err != nil || found || got != nil {
		t.Fatalf("late-child completion resurrected stale result = (%q, %v, %v)", got, found, err)
	}
}
