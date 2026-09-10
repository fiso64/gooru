package database

import (
	"database/sql"
	"testing"
	"time"
)

func TestCancelBackgroundOperationWithCleanupTaskPersistsDetachedTaskAfterLeaseHorizon(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 10, 20, 45, 0, 0, time.UTC)
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: "op-cleanup", Kind: "upload", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "upload-task", OperationID: "op-cleanup", DedupeKey: "upload", Kind: "upload.import", ResourceClass: "upload", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue upload = created %v err %v", created, err)
	}
	leaseDuration := time.Minute
	claimed, ok, err := store.ClaimNextBackgroundTask("upload", "worker-a", now, leaseDuration)
	if err != nil || !ok || claimed.ID != "upload-task" {
		t.Fatalf("claim upload = (%+v, %v, %v)", claimed, ok, err)
	}

	canceledAt := now.Add(10 * time.Second)
	result, err := store.CancelBackgroundOperationWithCleanupTask("op-cleanup", canceledAt, NewBackgroundTask{
		ID:            "cleanup-task",
		DedupeKey:     "upload-cleanup:op-cleanup",
		Kind:          "upload.cleanup",
		SubjectKind:   "operation",
		SubjectID:     "op-cleanup",
		ResourceClass: "upload",
		CreatedAt:     canceledAt,
		MaxAttempts:   5,
	})
	if err != nil || !result.Canceled || result.RunningTasks != 1 {
		t.Fatalf("cancel with cleanup = %+v, %v", result, err)
	}

	var operationStatus, uploadStatus string
	if err := store.DB.QueryRow(`SELECT status FROM background_operations WHERE id = 'op-cleanup'`).Scan(&operationStatus); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT status FROM background_tasks WHERE id = 'upload-task'`).Scan(&uploadStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "canceled" || uploadStatus != "canceled" {
		t.Fatalf("canceled state = operation %q upload %q", operationStatus, uploadStatus)
	}

	var cleanupOperationID sql.NullString
	var cleanupStatus string
	var cleanupAvailableAt int64
	if err := store.DB.QueryRow(`
		SELECT operation_id, status, available_at
		FROM background_tasks WHERE id = 'cleanup-task'
	`).Scan(&cleanupOperationID, &cleanupStatus, &cleanupAvailableAt); err != nil {
		t.Fatal(err)
	}
	if cleanupOperationID.Valid {
		t.Fatalf("cleanup operation_id = %q, want detached NULL", cleanupOperationID.String)
	}
	if cleanupStatus != "pending" {
		t.Fatalf("cleanup status = %q, want pending", cleanupStatus)
	}
	wantAvailableAt := workTimeValue(now.Add(leaseDuration))
	if cleanupAvailableAt != wantAvailableAt {
		t.Fatalf("cleanup available_at = %d, want revoked lease expiry %d", cleanupAvailableAt, wantAvailableAt)
	}

	if task, ok, err := store.ClaimNextBackgroundTask("upload", "worker-b", now.Add(59*time.Second), leaseDuration); err != nil || ok {
		t.Fatalf("early cleanup claim = (%+v, %v, %v), want none", task, ok, err)
	}
	cleanup, ok, err := store.ClaimNextBackgroundTask("upload", "worker-b", now.Add(leaseDuration), leaseDuration)
	if err != nil || !ok || cleanup.ID != "cleanup-task" {
		t.Fatalf("cleanup claim at old lease expiry = (%+v, %v, %v)", cleanup, ok, err)
	}
}

func TestCancelBackgroundOperationWithCleanupTaskRollsBackCancellationWhenCleanupCannotPersist(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 10, 20, 50, 0, 0, time.UTC)
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: "op-cleanup-rollback", Kind: "upload", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "upload-rollback", OperationID: "op-cleanup-rollback", DedupeKey: "upload-rollback", Kind: "upload.import", ResourceClass: "upload", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue upload = created %v err %v", created, err)
	}

	if _, err := store.CancelBackgroundOperationWithCleanupTask("op-cleanup-rollback", now.Add(time.Second), NewBackgroundTask{
		ID: "invalid-cleanup", Kind: "upload.cleanup", ResourceClass: "upload",
	}); err == nil {
		t.Fatal("cancel with invalid cleanup unexpectedly succeeded")
	}

	var operationStatus, taskStatus string
	if err := store.DB.QueryRow(`SELECT status FROM background_operations WHERE id = 'op-cleanup-rollback'`).Scan(&operationStatus); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT status FROM background_tasks WHERE id = 'upload-rollback'`).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "pending" || taskStatus != "pending" {
		t.Fatalf("rollback state = operation %q task %q, want pending/pending", operationStatus, taskStatus)
	}
	var cleanupCount int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM background_tasks WHERE id = 'invalid-cleanup'`).Scan(&cleanupCount); err != nil {
		t.Fatal(err)
	}
	if cleanupCount != 0 {
		t.Fatalf("invalid cleanup rows = %d, want 0", cleanupCount)
	}
}
