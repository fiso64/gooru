package database

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestReorderSavedSearchesBatchesOrderWrites(t *testing.T) {
	store := newMemoryTestStore(t)
	const userID = "usr_batch_order"
	if _, err := store.Exec(
		`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`,
		userID, "batch-order", "unused-test-hash", "admin",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	count := maxVars/3 + 1
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("search-%03d", i)
		if _, err := store.Exec(
			`INSERT INTO saved_searches (id, user_id, name, query, sort, "order") VALUES (?, ?, ?, ?, ?, ?)`,
			id, userID, id, "", "name", "asc",
		); err != nil {
			t.Fatalf("insert saved search %d: %v", i, err)
		}
		ids[count-1-i] = id
	}

	var logs bytes.Buffer
	store.logger.SetOutput(&logs)
	if err := store.ReorderSavedSearches(userID, ids); err != nil {
		t.Fatalf("ReorderSavedSearches: %v", err)
	}
	if got := strings.Count(logs.String(), "INSERT INTO saved_search_order"); got != 2 {
		t.Fatalf("saved-search order insert statements = %d, want 2", got)
	}

	items, err := store.ListSavedSearchesOrdered(userID)
	if err != nil {
		t.Fatalf("ListSavedSearchesOrdered: %v", err)
	}
	if len(items) != len(ids) {
		t.Fatalf("ordered searches = %d, want %d", len(items), len(ids))
	}
	for i, id := range ids {
		if items[i].ID != id {
			t.Fatalf("order[%d] = %q, want %q", i, items[i].ID, id)
		}
	}
}
