package database

import (
	"strings"
	"testing"
)

func TestVisibleBackgroundOperationListPlanUsesNewestIndex(t *testing.T) {
	store, _ := newDurableWorkTestDB(t)

	rows, err := store.Query(`
		EXPLAIN QUERY PLAN
		SELECT id, kind, visible, status, progress_total, progress_completed, progress_failed,
		       created_at, started_at, finished_at, error_code, error_message
		FROM background_operations
		WHERE visible = 1
		ORDER BY created_at DESC, id DESC
		LIMIT ?
	`, defaultBackgroundOperationListLimit)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	usedNewestIndex := false
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "background_operations_visible_created_idx") {
			usedNewestIndex = true
		}
		if strings.Contains(detail, "USE TEMP B-TREE FOR ORDER BY") {
			t.Fatalf("visible background operation list still materializes a newest-first sort: %s", detail)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !usedNewestIndex {
		t.Fatal("visible background operation list plan did not use background_operations_visible_created_idx")
	}
}
