package database

import (
	"database/sql"
	"io"
	"log"
	"strings"
	"testing"
)

func TestListNamespaceSuggestionsUsesDistinctPerKeyCounts(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}

	insertContent := func(hash string) {
		t.Helper()
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

	insertContent("a")
	insertContent("b")
	alice := insertTag("artist", "alice")
	bob := insertTag("artist", "bob")
	series := insertTag("series", "gooru")
	associate("a", alice)
	associate("a", bob) // same content/key must count once for the namespace.
	associate("b", alice)
	associate("a", series)

	items, err := store.ListNamespaceSuggestions("art", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Tag != "artist:" || items[0].Count != 2 {
		t.Fatalf("ListNamespaceSuggestions(art) = %#v, want artist: count 2", items)
	}
}

func TestNamespaceSuggestionPlanDoesNotReadTagsOrAssociations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT key || ':' AS tag_str, files_count
		FROM tag_key_counts
		WHERE files_count > 0 AND lower(key) LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?`, "art%", 20)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.ToLower(strings.Join(details, "\n"))
	if strings.Contains(plan, "content_tags") || strings.Contains(plan, "scan tags") {
		t.Fatalf("namespace suggestion query still reads tag-value/association tables:\n%s", plan)
	}
	if !strings.Contains(plan, "tag_key_counts") {
		t.Fatalf("namespace suggestion query does not use tag_key_counts:\n%s", plan)
	}
}
