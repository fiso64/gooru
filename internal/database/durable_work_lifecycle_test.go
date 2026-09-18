package database

import (
	"errors"
	"io"
	"log"
	"testing"
	"time"
)

func newDurableLifecycleTestStore(t *testing.T) *Store {
	t.Helper()
	store, db := newDurableWorkTestDB(t)
	db.SetMaxOpenConns(1)
	store.DB = db
	store.logger = log.New(io.Discard, "", 0)
	return store
}

func TestClaimNextBackgroundTaskOrdersAndRecordsAttempt(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)

	for _, task := range []NewBackgroundTask{
		{ID: "low", DedupeKey: "low", Kind: "thumbnail", ResourceClass: "image", Priority: 1, CreatedAt: now},
		{ID: "high", DedupeKey: "high", Kind: "thumbnail", ResourceClass: "image", Priority: 20, CreatedAt: now},
		{ID: "future", DedupeKey: "future", Kind: "thumbnail", ResourceClass: "image", Priority: 100, CreatedAt: now, AvailableAt: now.Add(time.Hour)},
		{ID: "other", DedupeKey: "other", Kind: "embedding", ResourceClass: "ml", Priority: 100, CreatedAt: now},
	} {
		if _, created, err := store.EnqueueBackgroundTask(store.DB, task); err != nil || !created {
			t.Fatalf("enqueue %s = created %v, err %v", task.ID, created, err)
		}
	}

	claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, 2*time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextBackgroundTask: %v", err)
	}
	if !ok || claimed.ID != "high" {
		t.Fatalf("claimed = (%+v, %v), want high", claimed, ok)
	}
	if claimed.Status != BackgroundWorkRunning || claimed.LeaseOwner != "worker-a" || claimed.AttemptCount != 1 {
		t.Fatalf("unexpected claimed task: %+v", claimed)
	}
	if claimed.LeaseExpiresAt == nil || !claimed.LeaseExpiresAt.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("lease expiry = %v, want %v", claimed.LeaseExpiresAt, now.Add(2*time.Minute))
	}

	var attemptWorker, outcome string
	var attemptStarted int64
	if err := store.DB.QueryRow(`
		SELECT worker_id, outcome, started_at
		FROM background_task_attempts WHERE task_id = 'high' AND attempt_number = 1
	`).Scan(&attemptWorker, &outcome, &attemptStarted); err != nil {
		t.Fatal(err)
	}
	if attemptWorker != "worker-a" || outcome != "running" || attemptStarted != workTimeValue(now) {
		t.Fatalf("attempt = worker %q outcome %q started %d", attemptWorker, outcome, attemptStarted)
	}

	second, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", now, time.Minute)
	if err != nil || !ok || second.ID != "low" {
		t.Fatalf("second claim = (%+v, %v, %v), want low", second, ok, err)
	}
	if _, ok, err := store.ClaimNextBackgroundTask("image", "worker-c", now, time.Minute); err != nil || ok {
		t.Fatalf("third immediate image claim = ok %v err %v, want none", ok, err)
	}
}

func TestBackgroundTaskFailureRetriesThenBecomesTerminal(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "retry-task", DedupeKey: "retry", Kind: "thumbnail", ResourceClass: "image", MaxAttempts: 2, CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}

	claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok || claimed.ID != "retry-task" {
		t.Fatalf("first claim = (%+v, %v, %v)", claimed, ok, err)
	}
	retryAt := now.Add(5 * time.Minute)
	willRetry, err := store.FailBackgroundTask(claimed.ID, "worker-a", now.Add(time.Second), retryAt, "decode", "temporary decoder failure")
	if err != nil || !willRetry {
		t.Fatalf("first failure = retry %v, err %v", willRetry, err)
	}

	var status string
	var availableAt int64
	var finishedAt interface{}
	if err := store.DB.QueryRow(`SELECT status, available_at, finished_at FROM background_tasks WHERE id = ?`, claimed.ID).Scan(&status, &availableAt, &finishedAt); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || availableAt != workTimeValue(retryAt) || finishedAt != nil {
		t.Fatalf("after retryable failure: status %q available %d finished %v", status, availableAt, finishedAt)
	}
	if _, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", retryAt.Add(-time.Millisecond), time.Minute); err != nil || ok {
		t.Fatalf("early retry claim = ok %v err %v, want none", ok, err)
	}

	claimed, ok, err = store.ClaimNextBackgroundTask("image", "worker-b", retryAt, time.Minute)
	if err != nil || !ok || claimed.AttemptCount != 2 {
		t.Fatalf("second claim = (%+v, %v, %v)", claimed, ok, err)
	}
	willRetry, err = store.FailBackgroundTask(claimed.ID, "worker-b", retryAt.Add(time.Second), time.Time{}, "decode", "permanent decoder failure")
	if err != nil || willRetry {
		t.Fatalf("terminal failure = retry %v, err %v", willRetry, err)
	}

	var finalStatus string
	var finalFinished int64
	if err := store.DB.QueryRow(`SELECT status, finished_at FROM background_tasks WHERE id = ?`, claimed.ID).Scan(&finalStatus, &finalFinished); err != nil {
		t.Fatal(err)
	}
	if finalStatus != "failed" || finalFinished != workTimeValue(retryAt.Add(time.Second)) {
		t.Fatalf("terminal task = status %q finished %d", finalStatus, finalFinished)
	}
	var failedAttempts int
	if err := store.DB.QueryRow(`SELECT count(*) FROM background_task_attempts WHERE task_id = ? AND outcome = 'failed'`, claimed.ID).Scan(&failedAttempts); err != nil {
		t.Fatal(err)
	}
	if failedAttempts != 2 {
		t.Fatalf("failed attempts = %d, want 2", failedAttempts)
	}
}

func TestCompleteBackgroundTaskRejectsStaleWorker(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "complete-task", DedupeKey: "complete", Kind: "thumbnail", ResourceClass: "image", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}
	claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim = (%+v, %v, %v)", claimed, ok, err)
	}
	if err := store.CompleteBackgroundTask(claimed.ID, "worker-b", now.Add(time.Second)); !errors.Is(err, ErrBackgroundTaskLeaseLost) {
		t.Fatalf("stale completion error = %v, want ErrBackgroundTaskLeaseLost", err)
	}
	if err := store.CompleteBackgroundTask(claimed.ID, "worker-a", now.Add(2*time.Second)); err != nil {
		t.Fatalf("owner completion: %v", err)
	}

	var taskStatus, attemptOutcome, leaseOwner string
	if err := store.DB.QueryRow(`SELECT status, lease_owner FROM background_tasks WHERE id = ?`, claimed.ID).Scan(&taskStatus, &leaseOwner); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT outcome FROM background_task_attempts WHERE task_id = ? AND attempt_number = 1`, claimed.ID).Scan(&attemptOutcome); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "completed" || attemptOutcome != "completed" || leaseOwner != "" {
		t.Fatalf("completion state = task %q attempt %q lease %q", taskStatus, attemptOutcome, leaseOwner)
	}
}

func TestCompleteBackgroundTaskTxRollsBackWithCaller(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{ID: "tx-complete", DedupeKey: "tx-complete", Kind: "thumbnail", ResourceClass: "image", CreatedAt: now}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}
	claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim = (%+v, %v, %v)", claimed, ok, err)
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteBackgroundTaskTx(tx, claimed.ID, "worker-a", now.Add(time.Second)); err != nil {
		_ = tx.Rollback()
		t.Fatalf("transaction completion: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback completion: %v", err)
	}
	var taskStatus, attemptOutcome string
	if err := store.DB.QueryRow(`SELECT status FROM background_tasks WHERE id = ?`, claimed.ID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT outcome FROM background_task_attempts WHERE task_id = ? AND attempt_number = 1`, claimed.ID).Scan(&attemptOutcome); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "running" || attemptOutcome != "running" {
		t.Fatalf("rolled-back completion leaked: task=%q attempt=%q", taskStatus, attemptOutcome)
	}
}


func TestCompleteBackgroundTaskAttemptTxRejectsRecoveredClaim(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "claim-generation", DedupeKey: "claim-generation", Kind: "thumbnail", ResourceClass: "image", MaxAttempts: 3, CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}
	first, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok || first.AttemptCount != 1 {
		t.Fatalf("first claim = (%+v, %v, %v)", first, ok, err)
	}
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(now.Add(time.Minute)); err != nil || recovered != 1 {
		t.Fatalf("recover first claim = %d, %v", recovered, err)
	}
	secondStart := now.Add(time.Minute)
	second, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", secondStart, time.Minute)
	if err != nil || !ok || second.AttemptCount != 2 {
		t.Fatalf("second claim = (%+v, %v, %v)", second, ok, err)
	}

	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = store.CompleteBackgroundTaskAttemptTx(tx, first.ID, first.AttemptCount, secondStart.Add(time.Second))
	_ = tx.Rollback()
	if !errors.Is(err, ErrBackgroundTaskLeaseLost) {
		t.Fatalf("stale claim completion error = %v, want ErrBackgroundTaskLeaseLost", err)
	}
	if err := store.CompleteBackgroundTask(second.ID, "worker-b", secondStart.Add(2*time.Second)); err != nil {
		t.Fatalf("current claim completion: %v", err)
	}
}


func TestRecoverExpiredBackgroundTaskLeasesRetriesThenExhausts(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "lease-task", DedupeKey: "lease", Kind: "thumbnail", ResourceClass: "image", MaxAttempts: 2, CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}
	first, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("first claim = (%+v, %v, %v)", first, ok, err)
	}
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(now.Add(30 * time.Second)); err != nil || recovered != 0 {
		t.Fatalf("early recovery = %d, %v", recovered, err)
	}
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(now.Add(time.Minute)); err != nil || recovered != 1 {
		t.Fatalf("first recovery = %d, %v", recovered, err)
	}

	var status, attemptOutcome string
	if err := store.DB.QueryRow(`SELECT status FROM background_tasks WHERE id = ?`, first.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT outcome FROM background_task_attempts WHERE task_id = ? AND attempt_number = 1`, first.ID).Scan(&attemptOutcome); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attemptOutcome != "abandoned" {
		t.Fatalf("first recovery state = task %q attempt %q", status, attemptOutcome)
	}

	secondStart := now.Add(time.Minute)
	second, ok, err := store.ClaimNextBackgroundTask("image", "worker-b", secondStart, time.Minute)
	if err != nil || !ok || second.AttemptCount != 2 {
		t.Fatalf("second claim = (%+v, %v, %v)", second, ok, err)
	}
	if err := store.CompleteBackgroundTask(second.ID, "worker-a", secondStart.Add(time.Second)); !errors.Is(err, ErrBackgroundTaskLeaseLost) {
		t.Fatalf("old worker completion error = %v, want lease lost", err)
	}
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(secondStart.Add(time.Minute)); err != nil || recovered != 1 {
		t.Fatalf("terminal recovery = %d, %v", recovered, err)
	}

	var finalStatus, errorCode string
	if err := store.DB.QueryRow(`SELECT status, last_error_code FROM background_tasks WHERE id = ?`, second.ID).Scan(&finalStatus, &errorCode); err != nil {
		t.Fatal(err)
	}
	if finalStatus != "failed" || errorCode != "lease_expired" {
		t.Fatalf("final recovery state = status %q code %q", finalStatus, errorCode)
	}
	if _, ok, err := store.ClaimNextBackgroundTask("image", "worker-c", secondStart.Add(2*time.Minute), time.Minute); err != nil || ok {
		t.Fatalf("claim exhausted task = ok %v err %v, want none", ok, err)
	}
}
