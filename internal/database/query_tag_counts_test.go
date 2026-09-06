package database

import (
	"database/sql"
	"io"
	"log"
	"testing"
)

func TestBatchGetQueryTagCountsUsesNamespaceSummary(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}

	for _, hash := range []string{"a", "b"} {
		if _, err := db.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
	}
	insertTag := func(key, value string) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO tags (key, value) VALUES (?, ?)`, key, value)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	associate := func(hash string, tagID int64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`, hash, tagID); err != nil {
			t.Fatal(err)
		}
	}

	alice := insertTag("artist", "alice")
	bob := insertTag("artist", "bob")
	favorite := insertTag("favorite", "")
	associate("a", alice)
	associate("a", bob)
	associate("b", alice)
	associate("a", favorite)

	counts, err := store.BatchGetQueryTagCounts([]string{"artist:", "artist:alice", "favorite"})
	if err != nil {
		t.Fatal(err)
	}
	if got := counts["artist:"]; got != 2 {
		t.Fatalf("artist: count = %d, want 2", got)
	}
	if got := counts["artist:alice"]; got != 2 {
		t.Fatalf("artist:alice count = %d, want 2", got)
	}
	if got := counts["favorite"]; got != 1 {
		t.Fatalf("favorite count = %d, want 1", got)
	}
}
