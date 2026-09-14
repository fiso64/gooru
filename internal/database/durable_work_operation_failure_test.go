package database

import (
	"testing"
	"time"
)

func TestFailBackgroundOperationRevokesChildrenAndSchedulesCleanup(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 14, 1, 0, 0, 0, time.UTC)
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: "op-producer-fail", Kind: "upload_import", Visible: true, ProgressTotal: 2, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"segment-running", "segment-pending"} {
		if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
			ID: id, OperationID: "op-producer-fail", DedupeKey: id, Kind: "upload.import", ResourceClass: "upload", CreatedAt: now,
		}); err != nil || !created {
			t.Fatalf("enqueue %s = created %v err %v", id, created, err)
		}
	}
	claimed, ok, err := store.ClaimNextBackgroundTask("upload", "worker-a", now.Add(time.Second), time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim = (%+v, %v, %v)", claimed, ok, err)
	}

	failedAt := now.Add(2 * time.Second)
	failure, err := store.FailBackgroundOperationWithCleanupTask("op-producer-fail", failedAt, "upload_request_failed", "upload request failed", NewBackgroundTask{
		ID: "cleanup-producer-fail", DedupeKey: "upload-cleanup:op-producer-fail", Kind: "upload.cleanup", SubjectKind: "operation", SubjectID: "op-producer-fail", ResourceClass: "upload", MaxAttempts: 5,
	})
	if err != nil || !failure.Failed {
		t.Fatalf("failure = %+v err %v", failure, err)
	}
	if failure.RunningTasks != 1 {
		t.Fatalf("running tasks = %d, want 1", failure.RunningTasks)
	}

	var status, errorCode, errorMessage string
	if err := store.DB.QueryRow(`SELECT status, error_code, error_message FROM background_operations WHERE id = 'op-producer-fail'`).Scan(&status, &errorCode, &errorMessage); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errorCode != "upload_request_failed" || errorMessage != "upload request failed" {
		t.Fatalf("operation = status %q code %q message %q", status, errorCode, errorMessage)
	}
	for _, id := range []string{"segment-running", "segment-pending"} {
		var taskStatus string
		if err := store.DB.QueryRow(`SELECT status FROM background_tasks WHERE id = ?`, id).Scan(&taskStatus); err != nil {
			t.Fatal(err)
		}
		if taskStatus != "canceled" {
			t.Fatalf("task %s status = %q, want canceled", id, taskStatus)
		}
	}
	var cleanupStatus, cleanupOperationID string
	var cleanupAvailable int64
	if err := store.DB.QueryRow(`SELECT status, COALESCE(operation_id, ''), available_at FROM background_tasks WHERE id = 'cleanup-producer-fail'`).Scan(&cleanupStatus, &cleanupOperationID, &cleanupAvailable); err != nil {
		t.Fatal(err)
	}
	if cleanupStatus != "pending" || cleanupOperationID != "" {
		t.Fatalf("cleanup = status %q operation %q", cleanupStatus, cleanupOperationID)
	}
	leaseHorizon := workTimeValue(now.Add(time.Second + time.Minute))
	if cleanupAvailable < leaseHorizon {
		t.Fatalf("cleanup available_at = %d, want >= %d", cleanupAvailable, leaseHorizon)
	}
	second, err := store.FailBackgroundOperation("op-producer-fail", failedAt.Add(time.Second), "later", "later")
	if err != nil || second {
		t.Fatalf("second failure = %v, %v, want unchanged", second, err)
	}
}

func TestFailBackgroundOperationSkipsCleanupWithoutChildren(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 14, 1, 5, 0, 0, time.UTC)
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: "op-pre-task-fail", Kind: "upload_import", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	failure, err := store.FailBackgroundOperationWithCleanupTask("op-pre-task-fail", now.Add(time.Second), "payload_too_large", "file exceeds configured limit", NewBackgroundTask{
		ID: "cleanup-pre-task-fail", DedupeKey: "upload-cleanup:op-pre-task-fail", Kind: "upload.cleanup", SubjectKind: "operation", SubjectID: "op-pre-task-fail", ResourceClass: "upload", MaxAttempts: 5,
	})
	if err != nil || !failure.Failed || failure.RunningTasks != 0 {
		t.Fatalf("failure = %+v err %v", failure, err)
	}
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM background_tasks WHERE id = 'cleanup-pre-task-fail'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("cleanup task count = %d, want 0", count)
	}
	var status, errorCode string
	if err := store.DB.QueryRow(`SELECT status, error_code FROM background_operations WHERE id = 'op-pre-task-fail'`).Scan(&status, &errorCode); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || errorCode != "payload_too_large" {
		t.Fatalf("operation = status %q code %q", status, errorCode)
	}
}
