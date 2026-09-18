package database

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
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


func TestGetLocationsForDirsBatchesQueriesWithinVariableBudget(t *testing.T) {
	store := newMemoryTestStore(t)
	dirs := make([]string, maxVars+1)
	for i := range dirs {
		dirs[i] = filepath.Join("scope", fmt.Sprintf("root-%03d", i))
	}

	const hash = "relink-scope-hash"
	if _, err := store.GetOrCreateContent(store, hash); err != nil {
		t.Fatalf("seed content: %v", err)
	}
	path := filepath.Join(dirs[maxVars], "file.jpg")
	if err := store.GetOrCreateLocation(store, hash, path, 1, 1, ".jpg"); err != nil {
		t.Fatalf("seed location: %v", err)
	}

	var queries bytes.Buffer
	store.logger = log.New(&queries, "", 0)
	locations, err := store.GetLocationsForDirs(dirs)
	if err != nil {
		t.Fatalf("GetLocationsForDirs: %v", err)
	}
	if _, ok := locations[path]; !ok {
		t.Fatalf("missing location from final root: %q", path)
	}
	if got := strings.Count(queries.String(), "WITH requested(pattern) AS"); got != 2 {
		t.Fatalf("scope queries = %d, want 2; log: %s", got, queries.String())
	}
}
