package database

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"strings"
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

	counts, err := store.BatchGetQueryTagCounts([]string{"artist", "artist:", "artist:alice", "favorite", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if got := counts["artist"]; got != 2 {
		t.Fatalf("artist count = %d, want 2", got)
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
	if got, ok := counts["missing"]; !ok || got != 0 {
		t.Fatalf("missing count = %d, present=%v, want explicit zero", got, ok)
	}
}

func TestBatchGetQueryTagCountsRespectsVariableLimit(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	var queryLog bytes.Buffer
	store := &Store{DB: db, logger: log.New(&queryLog, "", 0)}
	tags := make([]string, 0, maxVars/2+1+maxVars+1)
	for i := 0; i < maxVars/2+1; i++ {
		tags = append(tags, fmt.Sprintf("exact%d:value", i))
	}
	for i := 0; i < maxVars+1; i++ {
		tags = append(tags, fmt.Sprintf("key%d", i))
	}

	counts, err := store.BatchGetQueryTagCounts(tags)
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != len(tags) {
		t.Fatalf("count entries = %d, want %d", len(counts), len(tags))
	}

	logged := queryLog.String()
	if got := strings.Count(logged, "-- ARGS: 900 bound values redacted"); got != 2 {
		t.Fatalf("900-bind query count = %d, want 2; log:\n%s", got, logged)
	}
	if !strings.Contains(logged, "-- ARGS: 2 bound values redacted") {
		t.Fatalf("missing 2-bind exact-tag remainder query; log:\n%s", logged)
	}
	if !strings.Contains(logged, "-- ARGS: 1 bound values redacted") {
		t.Fatalf("missing 1-bind key remainder query; log:\n%s", logged)
	}
	if strings.Contains(logged, "-- ARGS: 902 bound values redacted") || strings.Contains(logged, "-- ARGS: 901 bound values redacted") {
		t.Fatalf("query exceeded maxVars; log:\n%s", logged)
	}
}
