package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"gooru.local/types"
)

type tagAssociationExecCountingQuerier struct {
	Querier
	associationExecs int
	maxExecArgs      int
}

func (q *tagAssociationExecCountingQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	if strings.Contains(query, "INSERT OR IGNORE INTO content_tags") {
		q.associationExecs++
		if len(args) > q.maxExecArgs {
			q.maxExecArgs = len(args)
		}
		if len(args) > maxVars {
			return nil, fmt.Errorf("tag association used %d bind variables, max %d", len(args), maxVars)
		}
	}
	return q.Querier.Exec(query, args...)
}

func TestBatchAssociateTagsByContentQueryTxBatchesTags(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-one", "hash-two", "hash-three"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	parsedTags := []types.ParsedTag{{Key: "one"}, {Key: "two"}, {Key: "three"}}
	tagIDMap, err := store.BatchGetOrCreateTags(store.DB, parsedTags)
	if err != nil {
		t.Fatalf("BatchGetOrCreateTags: %v", err)
	}
	tagIDs := make([]int64, 0, len(parsedTags))
	for _, tag := range parsedTags {
		tagIDs = append(tagIDs, tagIDMap[parsedTagString(tag)])
	}

	counting := &tagAssociationExecCountingQuerier{Querier: store.DB}
	affected, err := store.BatchAssociateTagsByContentQueryTx(
		counting,
		"SELECT hash FROM contents WHERE hash IN (?, ?)",
		[]interface{}{"hash-one", "hash-two"},
		tagIDs,
	)
	if err != nil {
		t.Fatalf("BatchAssociateTagsByContentQueryTx: %v", err)
	}
	if counting.associationExecs != 1 {
		t.Fatalf("tag association Exec calls = %d, want 1 for one bounded tag batch", counting.associationExecs)
	}
	if affected != 6 {
		t.Fatalf("affected associations = %d, want 6", affected)
	}

	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM content_tags WHERE content_hash IN ('hash-one', 'hash-two')`).Scan(&count); err != nil {
		t.Fatalf("count associations: %v", err)
	}
	if count != 6 {
		t.Fatalf("persisted associations = %d, want 6", count)
	}
	if err := store.QueryRow(`SELECT COUNT(*) FROM content_tags WHERE content_hash = 'hash-three'`).Scan(&count); err != nil {
		t.Fatalf("count excluded associations: %v", err)
	}
	if count != 0 {
		t.Fatalf("excluded content associations = %d, want 0", count)
	}
}

func TestBatchAssociateTagsByContentQueryTxRespectsVariableBudget(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-one"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	const subQueryArgs = 2
	tagCount := maxVars - subQueryArgs + 1
	parsedTags := make([]types.ParsedTag, tagCount)
	for i := range parsedTags {
		parsedTags[i] = types.ParsedTag{Key: fmt.Sprintf("assoc-%03d", i)}
	}
	tagIDMap, err := store.BatchGetOrCreateTags(store.DB, parsedTags)
	if err != nil {
		t.Fatalf("BatchGetOrCreateTags: %v", err)
	}
	tagIDs := make([]int64, 0, len(parsedTags))
	for _, tag := range parsedTags {
		tagIDs = append(tagIDs, tagIDMap[parsedTagString(tag)])
	}

	counting := &tagAssociationExecCountingQuerier{Querier: store.DB}
	affected, err := store.BatchAssociateTagsByContentQueryTx(
		counting,
		"SELECT hash FROM contents WHERE hash = ? OR hash = ?",
		[]interface{}{"hash-one", "missing"},
		tagIDs,
	)
	if err != nil {
		t.Fatalf("BatchAssociateTagsByContentQueryTx: %v", err)
	}
	if counting.associationExecs != 2 {
		t.Fatalf("tag association Exec calls = %d, want 2 for %d tags plus %d subquery args", counting.associationExecs, tagCount, subQueryArgs)
	}
	if counting.maxExecArgs > maxVars {
		t.Fatalf("largest tag association used %d bind variables, max %d", counting.maxExecArgs, maxVars)
	}
	if affected != int64(tagCount) {
		t.Fatalf("affected associations = %d, want %d", affected, tagCount)
	}
}