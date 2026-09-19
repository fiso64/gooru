package database

import (
	"database/sql"
	"io"
	"log"
	"strings"
	"testing"
)

func TestListTagSuggestionsPreservesPrefixSemantics(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}

	for _, item := range []struct {
		key   string
		value string
		count int
	}{
		{key: "article", value: "", count: 9},
		{key: "artist", value: "alice", count: 7},
		{key: "artist", value: "bob", count: 3},
		{key: "artwork", value: "modern", count: 5},
		{key: "series", value: "artist", count: 99},
	} {
		if _, err := db.Exec(`INSERT INTO tags (key, value, files_count) VALUES (?, ?, ?)`, item.key, item.value, item.count); err != nil {
			t.Fatal(err)
		}
	}

	items, err := store.ListTagSuggestions("ArT", 20)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(items))
	for _, item := range items {
		got = append(got, item.Tag)
	}
	want := []string{"article", "artist:alice", "artwork:modern", "artist:bob"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ListTagSuggestions(ArT) = %v, want %v", got, want)
	}

	items, err = store.ListTagSuggestions("artist:a", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Tag != "artist:alice" || items[0].Count != 7 {
		t.Fatalf("ListTagSuggestions(artist:a) = %#v, want artist:alice count 7", items)
	}
}

func TestListTagSuggestionsPreservesWildcardCompatibility(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}

	if _, err := db.Exec(`INSERT INTO tags (key, value, files_count) VALUES ('artist', 'alice', 7)`); err != nil {
		t.Fatal(err)
	}
	items, err := store.ListTagSuggestions("%ice", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Tag != "artist:alice" {
		t.Fatalf("ListTagSuggestions(%%ice) = %#v, want artist:alice", items)
	}
}

func TestTagSuggestionPrefixPlansUseTagKeyIndex(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	assertSearchPlan := func(query string, args ...any) {
		t.Helper()
		rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
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
		if strings.Contains(plan, "scan tags") {
			t.Fatalf("tag suggestion plan scans the full tags table:\n%s", plan)
		}
		if !strings.Contains(plan, "search tags") || !strings.Contains(plan, "sqlite_autoindex_tags_1") {
			t.Fatalf("tag suggestion plan does not range-search the key/value index:\n%s", plan)
		}
	}

	assertSearchPlan(`
		SELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
		FROM tags
		WHERE key LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?`, "art%", 20)

	assertSearchPlan(`
		SELECT CASE WHEN value = '' THEN key ELSE key || ':' || value END AS tag_str, files_count
		FROM tags
		WHERE key = ? AND value LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?`, "artist", "a%", 20)
}

func TestComponentCompletionRegression(t *testing.T) {
 db, err := sql.Open("sqlite3", ":memory:")
 if err != nil { t.Fatal(err) }
 defer db.Close()
 if err := RunMigrations(db); err != nil { t.Fatal(err) }
 store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}
 if _, err := db.Exec("INSERT INTO tags(key,value,files_count) VALUES('test_name','value_more',1)"); err != nil { t.Fatal(err) }
 for _, prefix := range []string{"test", "name", "val", "more"} {
  items, err := store.ListTagSuggestions(prefix, 10)
  if err != nil { t.Fatal(err) }
  if len(items) != 1 || items[0].Tag != "test_name:value_more" { t.Fatalf("prefix %q: %+v", prefix, items) }
 }
}
