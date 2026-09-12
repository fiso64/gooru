package database

import (
	"testing"
	"time"
)

func TestClearTerminalBackgroundOperationsRemovesOnlySafeVisibleHistory(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	now := time.Date(2026, time.September, 12, 2, 0, 0, 0, time.UTC)

	operations := []struct {
		id      string
		visible bool
		status  BackgroundWorkStatus
	}{
		{id: "completed", visible: true, status: BackgroundWorkCompleted},
		{id: "failed", visible: true, status: BackgroundWorkFailed},
		{id: "canceled", visible: true, status: BackgroundWorkCanceled},
		{id: "pending", visible: true, status: BackgroundWorkPending},
		{id: "running", visible: true, status: BackgroundWorkRunning},
		{id: "hidden-completed", visible: false, status: BackgroundWorkCompleted},
		{id: "terminal-with-active-child", visible: true, status: BackgroundWorkCompleted},
	}
	for index, item := range operations {
		if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{
			ID: item.id, Kind: "test", Visible: item.visible, ProgressTotal: 1, CreatedAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("CreateBackgroundOperation(%s): %v", item.id, err)
		}
		if item.status != BackgroundWorkPending {
			finishedAt := workTimeValue(now.Add(time.Minute))
			if _, err := db.Exec(`UPDATE background_operations SET status = ?, finished_at = ? WHERE id = ?`, string(item.status), finishedAt, item.id); err != nil {
				t.Fatalf("set operation status %s: %v", item.id, err)
			}
		}
	}

	terminalTask, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID: "task-completed", OperationID: "completed", DedupeKey: "completed:task", Kind: "test", CreatedAt: now,
	})
	if err != nil || !created {
		t.Fatalf("enqueue terminal child = (%+v, %v, %v)", terminalTask, created, err)
	}
	if _, err := db.Exec(`UPDATE background_tasks SET status = 'completed', finished_at = ? WHERE id = ?`, workTimeValue(now.Add(time.Minute)), terminalTask.ID); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID: "task-active", OperationID: "terminal-with-active-child", DedupeKey: "active:task", Kind: "test", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue active child = (%v, %v)", created, err)
	}

	cleared, err := store.ClearTerminalBackgroundOperations()
	if err != nil {
		t.Fatalf("ClearTerminalBackgroundOperations: %v", err)
	}
	if cleared != 3 {
		t.Fatalf("cleared = %d, want 3", cleared)
	}

	for _, id := range []string{"completed", "failed", "canceled"} {
		if _, found, err := store.GetBackgroundOperation(id); err != nil || found {
			t.Fatalf("operation %s after clear = found %v, err %v", id, found, err)
		}
	}
	for _, id := range []string{"pending", "running", "hidden-completed", "terminal-with-active-child"} {
		if _, found, err := store.GetBackgroundOperation(id); err != nil || !found {
			t.Fatalf("operation %s after clear = found %v, err %v", id, found, err)
		}
	}
	var terminalTaskCount int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE id = ?`, terminalTask.ID).Scan(&terminalTaskCount); err != nil {
		t.Fatal(err)
	}
	if terminalTaskCount != 0 {
		t.Fatalf("terminal child task count = %d, want 0", terminalTaskCount)
	}
	var activeTaskCount int
	if err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE id = 'task-active'`).Scan(&activeTaskCount); err != nil {
		t.Fatal(err)
	}
	if activeTaskCount != 1 {
		t.Fatalf("active child task count = %d, want 1", activeTaskCount)
	}
}
