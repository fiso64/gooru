package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"gooru.local/types"
)

type tagBatchCountingQuerier struct {
	Querier
	lookupQueries int
	insertExecs   int
	maxLookupArgs int
	maxInsertArgs int
}

func (q *tagBatchCountingQuerier) Query(query string, args ...interface{}) (*sql.Rows, error) {
	if strings.Contains(query, "SELECT id, key, value FROM tags WHERE (key, value) IN") {
		q.lookupQueries++
		if len(args) > q.maxLookupArgs {
			q.maxLookupArgs = len(args)
		}
		if len(args) > maxVars {
			return nil, fmt.Errorf("tag lookup used %d bind variables, max %d", len(args), maxVars)
		}
	}
	return q.Querier.Query(query, args...)
}

func (q *tagBatchCountingQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	if strings.Contains(query, "INSERT OR IGNORE INTO tags (key, value) VALUES") {
		q.insertExecs++
		if len(args) > q.maxInsertArgs {
			q.maxInsertArgs = len(args)
		}
		if len(args) > maxVars {
			return nil, fmt.Errorf("tag insert used %d bind variables, max %d", len(args), maxVars)
		}
	}
	return q.Querier.Exec(query, args...)
}

func TestBatchGetTagsRespectsVariableBudget(t *testing.T) {
	store := newMemoryTestStore(t)
	const tagCount = maxVars/2 + 1
	parsedTags := make([]types.ParsedTag, tagCount)
	for i := range parsedTags {
		parsedTags[i] = types.ParsedTag{Key: fmt.Sprintf("batch-key-%03d", i)}
	}

	for _, index := range []int{0, maxVars/2 - 1, maxVars / 2} {
		tag := parsedTags[index]
		if _, err := store.Exec("INSERT INTO tags (key, value) VALUES (?, ?)", tag.Key, tag.Value); err != nil {
			t.Fatalf("seed tag %d: %v", index, err)
		}
	}

	counting := &tagBatchCountingQuerier{Querier: store.DB}
	got, err := store.BatchGetTags(counting, parsedTags)
	if err != nil {
		t.Fatalf("BatchGetTags: %v", err)
	}
	if counting.lookupQueries != 2 {
		t.Fatalf("tag lookup queries = %d, want 2 for %d tags", counting.lookupQueries, tagCount)
	}
	if counting.maxLookupArgs > maxVars {
		t.Fatalf("largest tag lookup used %d bind variables, max %d", counting.maxLookupArgs, maxVars)
	}
	if len(got) != 3 {
		t.Fatalf("found tag count = %d, want 3", len(got))
	}
	for _, index := range []int{0, maxVars/2 - 1, maxVars / 2} {
		key := parsedTags[index].Key
		if got[key] <= 0 {
			t.Fatalf("missing ID for seeded tag %q", key)
		}
	}
}

func TestBatchGetOrCreateTagsBatchesInserts(t *testing.T) {
	store := newMemoryTestStore(t)
	parsedTags := []types.ParsedTag{
		{Key: "batched-one"},
		{Key: "batched-two", Value: "value"},
		{Key: "batched-three"},
	}
	counting := &tagBatchCountingQuerier{Querier: store.DB}
	got, err := store.BatchGetOrCreateTags(counting, parsedTags)
	if err != nil {
		t.Fatalf("BatchGetOrCreateTags: %v", err)
	}
	if counting.insertExecs != 1 {
		t.Fatalf("tag insert Exec calls = %d, want 1 for one missing-tag batch", counting.insertExecs)
	}
	if counting.maxInsertArgs > maxVars {
		t.Fatalf("largest tag insert used %d bind variables, max %d", counting.maxInsertArgs, maxVars)
	}

	wantKeys := []string{"batched-one", "batched-two:value", "batched-three"}
	for _, key := range wantKeys {
		if got[key] <= 0 {
			t.Fatalf("missing ID for created tag %q", key)
		}
	}
	var count int
	if err := store.QueryRow("SELECT COUNT(*) FROM tags WHERE key LIKE 'batched-%'").Scan(&count); err != nil {
		t.Fatalf("count created tags: %v", err)
	}
	if count != len(parsedTags) {
		t.Fatalf("created tag count = %d, want %d", count, len(parsedTags))
	}
}
