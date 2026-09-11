package database

import (
	"strings"
	"testing"
	"time"
)

func TestBackgroundClaimPlanUsesPriorityOrderedSchedulingIndex(t *testing.T) {
	store, _ := newDurableWorkTestDB(t)
	now := time.Date(2026, time.September, 11, 6, 0, 0, 0, time.UTC)

	rows, err := store.Query(`
		EXPLAIN QUERY PLAN
		SELECT id
		FROM background_tasks
		WHERE resource_class = ?
		  AND status = 'pending'
		  AND available_at <= ?
		  AND attempt_count < max_attempts
		ORDER BY priority DESC, created_at, id
		LIMIT 1
	`, "upload", workTimeValue(now))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	usedSchedulingIndex := false
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "background_tasks_schedulable_idx") {
			usedSchedulingIndex = true
		}
		if strings.Contains(detail, "USE TEMP B-TREE FOR ORDER BY") {
			t.Fatalf("background claim plan still materializes a priority sort: %s", detail)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usedSchedulingIndex {
		t.Fatal("background claim plan did not use background_tasks_schedulable_idx")
	}
}
