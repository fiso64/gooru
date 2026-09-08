package database

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCancelBackgroundOperationCancelsActiveChildrenAndRejectsLateEnqueue(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 8, 5, 30, 0, 0, time.UTC)
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: "op-cancel", Kind: "bulk-import", Visible: true, ProgressTotal: 3, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a-complete", "b-running", "c-pending"} {
		if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
			ID: id, OperationID: "op-cancel", DedupeKey: id, Kind: "import", ResourceClass: "io", CreatedAt: now,
		}); err != nil || !created {
			t.Fatalf("enqueue %s = created %v, err %v", id, created, err)
		}
	}

	first, ok, err := store.ClaimNextBackgroundTask("io", "worker-a", now, time.Minute)
	if err != nil || !ok || first.ID != "a-complete" {
		t.Fatalf("first claim = (%+v, %v, %v)", first, ok, err)
	}
	if err := store.CompleteBackgroundTask(first.ID, "worker-a", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	second, ok, err := store.ClaimNextBackgroundTask("io", "worker-b", now.Add(2*time.Second), time.Minute)
	if err != nil || !ok || second.ID != "b-running" {
		t.Fatalf("second claim = (%+v, %v, %v)", second, ok, err)
	}

	canceledAt := now.Add(3 * time.Second)
	canceled, err := store.CancelBackgroundOperation("op-cancel", canceledAt)
	if err != nil || !canceled {
		t.Fatalf("CancelBackgroundOperation = %v, %v", canceled, err)
	}

	var status string
	var completed, failed int64
	var finishedAt int64
	if err := store.DB.QueryRow(`
		SELECT status, progress_completed, progress_failed, finished_at
		FROM background_operations WHERE id = 'op-cancel'
	`).Scan(&status, &completed, &failed, &finishedAt); err != nil {
		t.Fatal(err)
	}
	if status != "canceled" || completed != 1 || failed != 0 || finishedAt != workTimeValue(canceledAt) {
		t.Fatalf("operation = status %q completed %d failed %d finished %d", status, completed, failed, finishedAt)
	}

	for _, id := range []string{"b-running", "c-pending"} {
		var taskStatus, leaseOwner string
		var taskFinished int64
		if err := store.DB.QueryRow(`SELECT status, lease_owner, finished_at FROM background_tasks WHERE id = ?`, id).Scan(&taskStatus, &leaseOwner, &taskFinished); err != nil {
			t.Fatal(err)
		}
		if taskStatus != "canceled" || leaseOwner != "" || taskFinished != workTimeValue(canceledAt) {
			t.Fatalf("task %s = status %q lease %q finished %d", id, taskStatus, leaseOwner, taskFinished)
		}
	}
	var attemptOutcome string
	if err := store.DB.QueryRow(`SELECT outcome FROM background_task_attempts WHERE task_id = 'b-running' AND attempt_number = 1`).Scan(&attemptOutcome); err != nil {
		t.Fatal(err)
	}
	if attemptOutcome != "canceled" {
		t.Fatalf("running attempt outcome = %q, want canceled", attemptOutcome)
	}
	if err := store.CompleteBackgroundTask("b-running", "worker-b", canceledAt.Add(time.Second)); !errors.Is(err, ErrBackgroundTaskLeaseLost) {
		t.Fatalf("completion after cancellation = %v, want lease lost", err)
	}
	if _, ok, err := store.ClaimNextBackgroundTask("io", "worker-c", canceledAt.Add(time.Second), time.Minute); err != nil || ok {
		t.Fatalf("claim after cancellation = ok %v err %v, want none", ok, err)
	}

	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "late", OperationID: "op-cancel", DedupeKey: "late", Kind: "import", ResourceClass: "io", CreatedAt: canceledAt,
	}); err == nil || created || !strings.Contains(err.Error(), "background operation is canceled") {
		t.Fatalf("late enqueue = created %v err %v, want canceled-operation rejection", created, err)
	}
	if canceled, err := store.CancelBackgroundOperation("op-cancel", canceledAt.Add(time.Second)); err != nil || canceled {
		t.Fatalf("second operation cancel = %v, %v, want unchanged", canceled, err)
	}
}

func TestCancelBackgroundTaskRollsUpCanceledOperation(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 8, 5, 40, 0, 0, time.UTC)
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: "op-items", Kind: "delete", Visible: true, ProgressTotal: 2, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"keep", "cancel"} {
		if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
			ID: id, OperationID: "op-items", DedupeKey: "item-" + id, Kind: "delete", ResourceClass: "io", CreatedAt: now,
		}); err != nil || !created {
			t.Fatalf("enqueue %s = created %v err %v", id, created, err)
		}
	}
	if canceled, err := store.CancelBackgroundTask("cancel", now.Add(time.Second)); err != nil || !canceled {
		t.Fatalf("cancel task = %v, %v", canceled, err)
	}
	if canceled, err := store.CancelBackgroundTask("cancel", now.Add(2*time.Second)); err != nil || canceled {
		t.Fatalf("second cancel task = %v, %v, want unchanged", canceled, err)
	}

	claimed, ok, err := store.ClaimNextBackgroundTask("io", "worker-a", now.Add(2*time.Second), time.Minute)
	if err != nil || !ok || claimed.ID != "keep" {
		t.Fatalf("claim remaining = (%+v, %v, %v)", claimed, ok, err)
	}
	if err := store.CompleteBackgroundTask("keep", "worker-a", now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}

	var operationStatus string
	var progressCompleted, progressFailed int64
	if err := store.DB.QueryRow(`
		SELECT status, progress_completed, progress_failed
		FROM background_operations WHERE id = 'op-items'
	`).Scan(&operationStatus, &progressCompleted, &progressFailed); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "canceled" || progressCompleted != 1 || progressFailed != 0 {
		t.Fatalf("operation = status %q completed %d failed %d", operationStatus, progressCompleted, progressFailed)
	}
}
