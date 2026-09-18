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
	if strings.Contains(query, "SELECT") && (strings.Contains(query, "FROM tags") || strings.Contains(query, "JOIN tags")) {
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

func TestBatchGetTagCountsCrossesBindBoundary(t *testing.T) {
	store := newMemoryTestStore(t)
	const tagCount = maxVars/2 + 1
	parsedTags := make([]types.ParsedTag, tagCount)
	for i := range parsedTags {
		parsedTags[i] = types.ParsedTag{Key: fmt.Sprintf("count-key-%03d", i)}
		tag := parsedTags[i]
		if _, err := store.Exec("INSERT INTO tags (key, value) VALUES (?, ?)", tag.Key, tag.Value); err != nil {
			t.Fatalf("seed tag %d: %v", i, err)
		}
	}

	got, err := store.BatchGetTagCounts(parsedTags)
	if err != nil {
		t.Fatalf("BatchGetTagCounts: %v", err)
	}
	if len(got) != tagCount {
		t.Fatalf("tag counts = %d, want %d", len(got), tagCount)
	}
	for _, index := range []int{0, maxVars/2 - 1, maxVars / 2} {
		key := parsedTags[index].Key
		if _, ok := got[key]; !ok {
			t.Fatalf("missing count for seeded tag %q", key)
		}
	}
}

func TestBatchGetOrCreateTagsBatchesInserts(t *testing.T) {
	store := newMemoryTestStore(t)
	const tagCount = maxVars/2 + 1
	parsedTags := make([]types.ParsedTag, tagCount)
	for i := range parsedTags {
		parsedTags[i] = types.ParsedTag{Key: fmt.Sprintf("batched-%04d", i)}
	}

	counting := &tagBatchCountingQuerier{Querier: store.DB}
	got, err := store.BatchGetOrCreateTags(counting, parsedTags)
	if err != nil {
		t.Fatalf("BatchGetOrCreateTags: %v", err)
	}
	if counting.insertExecs != 2 {
		t.Fatalf("tag insert Exec calls = %d, want 2 for %d tags", counting.insertExecs, tagCount)
	}
	if counting.maxInsertArgs > maxVars {
		t.Fatalf("largest tag insert used %d bind variables, max %d", counting.maxInsertArgs, maxVars)
	}
	if len(got) != tagCount {
		t.Fatalf("created tag IDs = %d, want %d", len(got), tagCount)
	}
	for _, index := range []int{0, maxVars/2 - 1, maxVars / 2} {
		key := parsedTags[index].Key
		if got[key] <= 0 {
			t.Fatalf("missing ID for created tag %q", key)
		}
	}

	var count int
	if err := store.QueryRow("SELECT COUNT(*) FROM tags WHERE key LIKE 'batched-%'").Scan(&count); err != nil {
		t.Fatalf("count created tags: %v", err)
	}
	if count != tagCount {
		t.Fatalf("created tag count = %d, want %d", count, tagCount)
	}
}

func TestBatchGetOrCreateTagsUsesInputSpellingForExistingTag(t *testing.T) {
	store := newMemoryTestStore(t)
	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback()

	mixedResult, err := tx.Exec("INSERT INTO tags (key, value) VALUES (?, ?)", "MiXeD", "VaLuE")
	if err != nil {
		t.Fatalf("insert mixed-case tag: %v", err)
	}
	mixedID, err := mixedResult.LastInsertId()
	if err != nil {
		t.Fatalf("mixed-case tag ID: %v", err)
	}
	if _, err := tx.Exec("INSERT INTO tags (key, value) VALUES (?, ?)", "unrelated", ""); err != nil {
		t.Fatalf("insert unrelated tag: %v", err)
	}

	got, err := store.BatchGetOrCreateTags(tx, []types.ParsedTag{{Key: "mixed", Value: "value"}})
	if err != nil {
		t.Fatalf("BatchGetOrCreateTags: %v", err)
	}
	if got["mixed:value"] != mixedID {
		t.Fatalf("ID for input spelling = %d, want existing case-insensitive tag ID %d", got["mixed:value"], mixedID)
	}
}
