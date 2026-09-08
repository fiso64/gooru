package database

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

type terminalizeDedupeConflictQuerier struct {
	db           *sql.DB
	transitioned bool
}

func (q *terminalizeDedupeConflictQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	res, err := q.db.Exec(query, args...)
	if err != nil || q.transitioned || !strings.Contains(query, "INSERT INTO background_tasks") {
		return res, err
	}
	rows, rowsErr := res.RowsAffected()
	if rowsErr != nil || rows != 0 {
		return res, err
	}
	q.transitioned = true
	_, transitionErr := q.db.Exec(`
		UPDATE background_tasks
		SET status = 'completed', finished_at = ?
		WHERE dedupe_key = ? AND status IN ('pending', 'running')
	`, workTimeValue(time.Now().UTC()), args[2])
	if transitionErr != nil {
		return nil, transitionErr
	}
	return res, nil
}

func (q *terminalizeDedupeConflictQuerier) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return q.db.Query(query, args...)
}

func (q *terminalizeDedupeConflictQuerier) QueryRow(query string, args ...interface{}) *sql.Row {
	return q.db.QueryRow(query, args...)
}

func TestEnqueueBackgroundTaskRetriesWhenConflictFinishesBeforeReadback(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	now := time.Date(2026, time.September, 7, 22, 0, 0, 0, time.UTC)

	first, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
		ID:        "task-race-first",
		DedupeKey: "thumbnail:file-race",
		Kind:      "thumbnail",
		CreatedAt: now,
	})
	if err != nil || !created {
		t.Fatalf("first enqueue = (%+v, %v, %v), want created", first, created, err)
	}

	q := &terminalizeDedupeConflictQuerier{db: db}
	second, created, err := store.EnqueueBackgroundTask(q, NewBackgroundTask{
		ID:        "task-race-second",
		DedupeKey: first.DedupeKey,
		Kind:      "thumbnail",
		CreatedAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("enqueue across terminal transition: %v", err)
	}
	if !q.transitioned {
		t.Fatal("test did not exercise the conflict/readback race")
	}
	if !created || second.ID != "task-race-second" {
		t.Fatalf("enqueue after disappearing conflict = (%+v, %v), want newly created task", second, created)
	}

	var active int
	if err := db.QueryRow(`
		SELECT count(*) FROM background_tasks
		WHERE dedupe_key = ? AND status IN ('pending', 'running')
	`, first.DedupeKey).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active task count = %d, want 1", active)
	}
}
