package database

import (
	"database/sql"
	"fmt"
	"testing"

	"gooru.local/types"
)

type relinkBindBudgetQuerier struct {
	*Tx
	maxArgsSeen int
}

func (q *relinkBindBudgetQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	if len(args) > q.maxArgsSeen {
		q.maxArgsSeen = len(args)
	}
	if len(args) > maxVars {
		return nil, fmt.Errorf("relink insert used %d bind variables, max %d", len(args), maxVars)
	}
	return q.Tx.Exec(query, args...)
}

func TestApplyRelinkAdditionsTxRespectsVariableBudget(t *testing.T) {
	store := newMemoryTestStore(t)
	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback()

	const columns = 5 // content_hash, path, size_bytes, mod_time, extension
	const additions = maxVars/columns + 1
	hashes := make([]string, additions)
	toAdd := make(map[string]types.LocationInfo, additions)
	for i := 0; i < additions; i++ {
		hash := fmt.Sprintf("relink-hash-%03d", i)
		path := fmt.Sprintf("/relinked/file-%03d.jpg", i)
		hashes[i] = hash
		toAdd[path] = types.LocationInfo{
			Hash:      hash,
			Size:      int64(i + 1),
			ModTime:   int64(1000 + i),
			Extension: ".jpg",
		}
	}
	if err := store.BatchInsertContents(tx, hashes); err != nil {
		t.Fatalf("seed contents: %v", err)
	}

	counting := &relinkBindBudgetQuerier{Tx: tx}
	added, err := store.ApplyRelinkAdditionsTx(counting, toAdd)
	if err != nil {
		t.Fatalf("ApplyRelinkAdditionsTx: %v", err)
	}
	if added != additions {
		t.Fatalf("locations added = %d, want %d", added, additions)
	}
	if counting.maxArgsSeen > maxVars {
		t.Fatalf("largest relink insert used %d bind variables, max %d", counting.maxArgsSeen, maxVars)
	}
}
