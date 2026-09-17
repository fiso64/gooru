package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"gooru.local/types"
)

type tagDisassociationExecCountingQuerier struct {
	Querier
	disassociationExecs int
	maxExecArgs         int
}

func (q *tagDisassociationExecCountingQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	if strings.Contains(query, "DELETE FROM content_tags WHERE tag_id IN") {
		q.disassociationExecs++
		if len(args) > q.maxExecArgs {
			q.maxExecArgs = len(args)
		}
		if len(args) > maxVars {
			return nil, fmt.Errorf("tag disassociation used %d bind variables, max %d", len(args), maxVars)
		}
	}
	return q.Querier.Exec(query, args...)
}

func TestBatchDisassociateTagsByContentQueryTxRespectsVariableBudget(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-one", "hash-two"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	const subQueryArgs = 2
	tagCount := maxVars - subQueryArgs + 1
	parsedTags := make([]types.ParsedTag, tagCount)
	for i := range parsedTags {
		parsedTags[i] = types.ParsedTag{Key: fmt.Sprintf("remove-%03d", i)}
	}
	tagIDMap, err := store.BatchGetOrCreateTags(store.DB, parsedTags)
	if err != nil {
		t.Fatalf("BatchGetOrCreateTags: %v", err)
	}
	tagIDs := make([]int64, 0, len(parsedTags))
	for _, tag := range parsedTags {
		tagIDs = append(tagIDs, tagIDMap[parsedTagString(tag)])
	}

	seeded, err := store.BatchAssociateTagsByContentQueryTx(
		store.DB,
		"SELECT hash FROM contents WHERE hash = ? OR hash = ?",
		[]interface{}{"hash-one", "hash-two"},
		tagIDs,
	)
	if err != nil {
		t.Fatalf("seed associations: %v", err)
	}
	if seeded != int64(tagCount*2) {
		t.Fatalf("seeded associations = %d, want %d", seeded, tagCount*2)
	}

	counting := &tagDisassociationExecCountingQuerier{Querier: store.DB}
	affected, err := store.BatchDisassociateTagsByContentQueryTx(
		counting,
		"SELECT hash FROM contents WHERE hash = ? OR hash = ?",
		[]interface{}{"hash-one", "missing"},
		tagIDs,
	)
	if err != nil {
		t.Fatalf("BatchDisassociateTagsByContentQueryTx: %v", err)
	}
	if counting.disassociationExecs != 2 {
		t.Fatalf("tag disassociation Exec calls = %d, want 2 for %d tags plus %d subquery args", counting.disassociationExecs, tagCount, subQueryArgs)
	}
	if counting.maxExecArgs > maxVars {
		t.Fatalf("largest tag disassociation used %d bind variables, max %d", counting.maxExecArgs, maxVars)
	}
	if affected != int64(tagCount) {
		t.Fatalf("affected associations = %d, want %d", affected, tagCount)
	}

	var count int
	if err := store.QueryRow(`SELECT COUNT(*) FROM content_tags WHERE content_hash = 'hash-one'`).Scan(&count); err != nil {
		t.Fatalf("count removed associations: %v", err)
	}
	if count != 0 {
		t.Fatalf("remaining hash-one associations = %d, want 0", count)
	}
	if err := store.QueryRow(`SELECT COUNT(*) FROM content_tags WHERE content_hash = 'hash-two'`).Scan(&count); err != nil {
		t.Fatalf("count excluded associations: %v", err)
	}
	if count != tagCount {
		t.Fatalf("remaining hash-two associations = %d, want %d", count, tagCount)
	}
}
