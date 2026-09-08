package database

import (
	"testing"
	"time"
)

func readBackgroundOperationLifecycle(t *testing.T, store *Store, operationID string) BackgroundOperation {
	t.Helper()
	var operation BackgroundOperation
	var status string
	var visible int
	var createdAt int64
	var startedAt, finishedAt nullableInt64
	if err := store.DB.QueryRow(`
		SELECT id, kind, visible, status, progress_total, progress_completed, progress_failed,
		       created_at, started_at, finished_at, error_code, error_message
		FROM background_operations
		WHERE id = ?
	`, operationID).Scan(
		&operation.ID, &operation.Kind, &visible, &status, &operation.ProgressTotal,
		&operation.ProgressCompleted, &operation.ProgressFailed, &createdAt,
		&startedAt, &finishedAt, &operation.ErrorCode, &operation.ErrorMessage,
	); err != nil {
		t.Fatalf("read background operation %s: %v", operationID, err)
	}
	operation.Visible = visible != 0
	operation.Status = BackgroundWorkStatus(status)
	operation.CreatedAt = workTime(createdAt)
	operation.StartedAt = nullableWorkTime(startedAt.NullInt64)
	operation.FinishedAt = nullableWorkTime(finishedAt.NullInt64)
	return operation
}

type nullableInt64 struct {
	NullInt64
}

func TestBackgroundOperationTracksChildTaskLifecycle(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 8, 1, 30, 0, 0, time.UTC)
	operationID := "operation-lifecycle"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "bulk-import", Visible: true, ProgressTotal: 2, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create operation: %v", err)
	}
	for _, task := range []NewBackgroundTask{
		{ID: "op-task-1", OperationID: operationID, DedupeKey: "op-task-1", Kind: "thumbnail", ResourceClass: "image", Priority: 2, CreatedAt: now},
		{ID: "op-task-2", OperationID: operationID, DedupeKey: "op-task-2", Kind: "thumbnail", ResourceClass: "image", Priority: 1, CreatedAt: now},
	} {
		if _, created, err := store.EnqueueBackgroundTask(store.DB, task); err != nil || !created {
			t.Fatalf("enqueue %s = created %v err %v", task.ID, created, err)
		}
	}

	first, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now.Add(time.Second), time.Minute)
	if err != nil || !ok || first.ID != "op-task-1" {
		t.Fatalf("first claim = (%+v, %v, %v)", first, ok, err)
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkRunning || op.ProgressCompleted != 0 || op.ProgressFailed != 0 || op.StartedAt == nil || !op.StartedAt.Equal(now.Add(time.Second)) || op.FinishedAt != nil {
		t.Fatalf("operation after claim = %+v", op)
	}

	firstFinished := now.Add(2 * time.Second)
	if err := store.CompleteBackgroundTask(first.ID, "worker-a", firstFinished); err != nil {
		t.Fatalf("complete first task: %v", err)
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkRunning || op.ProgressCompleted != 1 || op.ProgressFailed != 0 || op.FinishedAt != nil {
		t.Fatalf("operation after first completion = %+v", op)
	}

	second, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", now.Add(3*time.Second), time.Minute)
	if err != nil || !ok || second.ID != "op-task-2" {
		t.Fatalf("second claim = (%+v, %v, %v)", second, ok, err)
	}
	finalFinished := now.Add(4 * time.Second)
	if err := store.CompleteBackgroundTask(second.ID, "worker-b", finalFinished); err != nil {
		t.Fatalf("complete second task: %v", err)
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkCompleted || op.ProgressCompleted != 2 || op.ProgressFailed != 0 || op.FinishedAt == nil || !op.FinishedAt.Equal(finalFinished) {
		t.Fatalf("terminal operation = %+v", op)
	}
	if op.StartedAt == nil || !op.StartedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("operation start changed: %v", op.StartedAt)
	}
}

func TestBackgroundOperationRetryIsNonTerminalUntilTaskExhaustion(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 8, 1, 40, 0, 0, time.UTC)
	operationID := "operation-retry"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "thumbnail-backfill", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "retry-child", OperationID: operationID, DedupeKey: "retry-child", Kind: "thumbnail", ResourceClass: "image", MaxAttempts: 2, CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v err %v", created, err)
	}

	first, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim = (%+v, %v, %v)", first, ok, err)
	}
	retryAt := now.Add(2 * time.Minute)
	if retry, err := store.FailBackgroundTask(first.ID, "worker-a", now.Add(time.Second), retryAt, "decode", "transient"); err != nil || !retry {
		t.Fatalf("retryable failure = retry %v err %v", retry, err)
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkRunning || op.ProgressCompleted != 0 || op.ProgressFailed != 0 || op.FinishedAt != nil || op.ErrorCode != "" {
		t.Fatalf("operation after retryable failure = %+v", op)
	}

	second, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", retryAt, time.Minute)
	if err != nil || !ok {
		t.Fatalf("second claim = (%+v, %v, %v)", second, ok, err)
	}
	terminalAt := retryAt.Add(time.Second)
	if retry, err := store.FailBackgroundTask(second.ID, "worker-b", terminalAt, time.Time{}, "decode", "permanent"); err != nil || retry {
		t.Fatalf("terminal failure = retry %v err %v", retry, err)
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkFailed || op.ProgressCompleted != 0 || op.ProgressFailed != 1 || op.FinishedAt == nil || !op.FinishedAt.Equal(terminalAt) || op.ErrorCode != "child_task_failed" {
		t.Fatalf("operation after terminal failure = %+v", op)
	}
}

func TestBackgroundOperationLeaseRecoveryOnlyFailsAfterExhaustion(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 8, 1, 50, 0, 0, time.UTC)
	operationID := "operation-expiry"
	if _, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{
		ID: operationID, Kind: "thumbnail-backfill", Visible: true, ProgressTotal: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("create operation: %v", err)
	}
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "expiry-child", OperationID: operationID, DedupeKey: "expiry-child", Kind: "thumbnail", ResourceClass: "image", MaxAttempts: 2, CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v err %v", created, err)
	}

	if _, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute); err != nil || !ok {
		t.Fatalf("first claim = ok %v err %v", ok, err)
	}
	firstExpiry := now.Add(time.Minute)
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(firstExpiry); err != nil || recovered != 1 {
		t.Fatalf("first recovery = %d err %v", recovered, err)
	}
	op := readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkRunning || op.ProgressFailed != 0 || op.FinishedAt != nil {
		t.Fatalf("operation after retryable expiry = %+v", op)
	}

	if _, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", firstExpiry, time.Minute); err != nil || !ok {
		t.Fatalf("second claim = ok %v err %v", ok, err)
	}
	terminalExpiry := firstExpiry.Add(time.Minute)
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(terminalExpiry); err != nil || recovered != 1 {
		t.Fatalf("terminal recovery = %d err %v", recovered, err)
	}
	op = readBackgroundOperationLifecycle(t, store, operationID)
	if op.Status != BackgroundWorkFailed || op.ProgressFailed != 1 || op.FinishedAt == nil || !op.FinishedAt.Equal(terminalExpiry) || op.ErrorCode != "child_task_failed" {
		t.Fatalf("operation after terminal expiry = %+v", op)
	}
}
