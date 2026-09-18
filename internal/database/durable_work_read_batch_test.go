package database

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
)

func TestGetBackgroundOperationsRespectsBindBudget(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	ids := make([]string, maxVars+1)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed transaction: %v", err)
	}
	defer tx.Rollback()
	for i := range ids {
		ids[i] = fmt.Sprintf("operation-%04d", i)
		if _, err := store.CreateBackgroundOperation(tx, NewBackgroundOperation{
			ID:      ids[i],
			Kind:    "batch-read",
			Visible: true,
		}); err != nil {
			t.Fatalf("create operation %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed transaction: %v", err)
	}

	var queryLog bytes.Buffer
	store.logger = log.New(&queryLog, "", 0)
	operations, err := store.GetBackgroundOperations(ids)
	if err != nil {
		t.Fatalf("GetBackgroundOperations: %v", err)
	}
	if len(operations) != len(ids) {
		t.Fatalf("operations = %d, want %d", len(operations), len(ids))
	}

	logged := queryLog.String()
	if got := strings.Count(logged, "FROM background_operations"); got != 2 {
		t.Fatalf("background operation queries = %d, want 2\n%s", got, logged)
	}
	if !strings.Contains(logged, fmt.Sprintf("-- ARGS: %d bound values redacted", maxVars)) {
		t.Fatalf("missing max-sized batch in query log:\n%s", logged)
	}
	if !strings.Contains(logged, "-- ARGS: 1 bound values redacted") {
		t.Fatalf("missing remainder batch in query log:\n%s", logged)
	}
}
