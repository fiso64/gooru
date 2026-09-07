package database

import (
	"database/sql"
	"io"
	"log"
	"reflect"
	"strings"
	"testing"
)

func TestKeySummaryReadsPreserveCountAndNamespaceSemantics(t *testing.T) {
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

	count, err := store.GetCountForKey("artist")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("artist key count = %d, want 2 distinct files", count)
	}
	count, err = store.GetCountForKey("missing")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("missing key count = %d, want 0", count)
	}

	namespaces, err := store.ListTagNamespaces()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(namespaces, []string{"artist"}) {
		t.Fatalf("namespaces = %#v, want only non-empty-value key artist", namespaces)
	}
}

func TestKeySummaryReadPlansDoNotReadContentAssociations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	queries := []string{
		`EXPLAIN QUERY PLAN SELECT files_count FROM tag_key_counts WHERE key = 'artist'`,
		`EXPLAIN QUERY PLAN
		 SELECT tk.key
		 FROM tag_key_counts tk
		 WHERE EXISTS (
		     SELECT 1 FROM tags t
		     WHERE t.key = tk.key AND t.value != ''
		 )
		 ORDER BY tk.key`,
	}
	for _, query := range queries {
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		var details []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			details = append(details, detail)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		plan := strings.ToLower(strings.Join(details, "\n"))
		if strings.Contains(plan, "content_tags") {
			t.Fatalf("key summary read still touches content_tags:\n%s", plan)
		}
	}
}
