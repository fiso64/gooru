package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"gooru.local/types"
)

type cartesianContentTagExecCountingQuerier struct {
	Querier
	associationExecs    int
	disassociationExecs int
	maxExecArgs         int
}

func (q *cartesianContentTagExecCountingQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	if strings.Contains(query, "INSERT OR IGNORE INTO content_tags") {
		q.associationExecs++
	}
	if strings.Contains(query, "DELETE FROM content_tags WHERE (content_hash, tag_id) IN") {
		q.disassociationExecs++
	}
	if len(args) > q.maxExecArgs {
		q.maxExecArgs = len(args)
	}
	if len(args) > maxVars {
		return nil, fmt.Errorf("content-tag batch used %d bind variables, max %d", len(args), maxVars)
	}
	return q.Querier.Exec(query, args...)
}

func seedCartesianContentTags(t *testing.T, store *Store) ([]string, []int64) {
	t.Helper()
	const hashCount = 151
	hashes := make([]string, hashCount)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("hash-%03d", i)
	}
	if err := store.BatchInsertContents(store.DB, hashes); err != nil {
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
	return hashes, tagIDs
}

func TestBatchAssociateTagsForContentHashesBoundsPairBatches(t *testing.T) {
	store := newMemoryTestStore(t)
	hashes, tagIDs := seedCartesianContentTags(t, store)
	wantAssociations := int64(len(hashes) * len(tagIDs))

	counting := &cartesianContentTagExecCountingQuerier{Querier: store.DB}
	affected, err := store.BatchAssociateTagsForContentHashes(counting, hashes, tagIDs)
	if err != nil {
		t.Fatalf("BatchAssociateTagsForContentHashes: %v", err)
	}
	if affected != wantAssociations {
		t.Fatalf("affected associations = %d, want %d", affected, wantAssociations)
	}
	if counting.associationExecs != 2 {
		t.Fatalf("association Exec calls = %d, want 2 for 453 pairs", counting.associationExecs)
	}
	if counting.maxExecArgs > maxVars {
		t.Fatalf("largest content-tag batch used %d bind variables, max %d", counting.maxExecArgs, maxVars)
	}

	var persisted int64
	if err := store.QueryRow("SELECT COUNT(*) FROM content_tags").Scan(&persisted); err != nil {
		t.Fatalf("count persisted associations: %v", err)
	}
	if persisted != wantAssociations {
		t.Fatalf("persisted associations = %d, want %d", persisted, wantAssociations)
	}
}

func TestBatchDisassociateTagsForContentHashesBoundsPairBatches(t *testing.T) {
	store := newMemoryTestStore(t)
	hashes, tagIDs := seedCartesianContentTags(t, store)
	wantAssociations := int64(len(hashes) * len(tagIDs))
	if affected, err := store.BatchAssociateTagsForContentHashes(store.DB, hashes, tagIDs); err != nil {
		t.Fatalf("seed associations: %v", err)
	} else if affected != wantAssociations {
		t.Fatalf("seeded associations = %d, want %d", affected, wantAssociations)
	}

	counting := &cartesianContentTagExecCountingQuerier{Querier: store.DB}
	affected, err := store.BatchDisassociateTagsForContentHashes(counting, hashes, tagIDs)
	if err != nil {
		t.Fatalf("BatchDisassociateTagsForContentHashes: %v", err)
	}
	if affected != wantAssociations {
		t.Fatalf("affected disassociations = %d, want %d", affected, wantAssociations)
	}
	if counting.disassociationExecs != 2 {
		t.Fatalf("disassociation Exec calls = %d, want 2 for 453 pairs", counting.disassociationExecs)
	}
	if counting.maxExecArgs > maxVars {
		t.Fatalf("largest content-tag batch used %d bind variables, max %d", counting.maxExecArgs, maxVars)
	}

	var remaining int64
	if err := store.QueryRow("SELECT COUNT(*) FROM content_tags").Scan(&remaining); err != nil {
		t.Fatalf("count remaining associations: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining associations = %d, want 0", remaining)
	}
}
