package database

import (
	"fmt"
	"testing"
)

func TestRemoveLocationsByPublicIDTxChunksBeyondSQLiteVariableLimit(t *testing.T) {
	store := newMemoryTestStore(t)
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, "shared-hash"); err != nil {
		t.Fatalf("insert content: %v", err)
	}

	count := maxVars + 17
	publicIDs := make([]string, 0, count)
	seed, err := store.Begin()
	if err != nil {
		t.Fatalf("begin seed transaction: %v", err)
	}
	defer seed.Rollback()
	for index := 0; index < count; index++ {
		publicID := fmt.Sprintf("file_batch_%06d", index)
		publicIDs = append(publicIDs, publicID)
		if _, err := seed.Exec(`
			INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
			VALUES (?, ?, ?, ?, ?, ?)
		`, publicID, "shared-hash", fmt.Sprintf("/library/%06d.jpg", index), index+1, index+10, ".jpg"); err != nil {
			t.Fatalf("insert location %d: %v", index, err)
		}
	}
	if err := seed.Commit(); err != nil {
		t.Fatalf("commit seed transaction: %v", err)
	}

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("begin delete transaction: %v", err)
	}
	defer tx.Rollback()
	removed, err := store.RemoveLocationsByPublicIDTx(tx, publicIDs)
	if err != nil {
		t.Fatalf("RemoveLocationsByPublicIDTx: %v", err)
	}
	if removed != count {
		t.Fatalf("removed=%d want %d", removed, count)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit delete transaction: %v", err)
	}

	var remaining int
	if err := store.QueryRow(`SELECT COUNT(*) FROM locations`).Scan(&remaining); err != nil {
		t.Fatalf("count remaining locations: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("remaining locations=%d want 0", remaining)
	}
}
