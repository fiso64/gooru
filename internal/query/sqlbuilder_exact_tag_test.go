package query

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	_ "gosqlite.org"
)

func TestBuildExactTagUsesAssociationIndexPath(t *testing.T) {
	expr, err := Parse("artist:alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAST(expr); err != nil {
		t.Fatal(err)
	}

	contentQuery, contentArgs := Build(expr, nil)
	if strings.Contains(contentQuery, "JOIN locations") || strings.Contains(contentQuery, "DISTINCT") {
		t.Fatalf("exact content tag query should not visit locations or deduplicate: %s", contentQuery)
	}
	if !strings.Contains(contentQuery, "ct.tag_id = (SELECT id FROM tags") {
		t.Fatalf("exact content tag query should resolve one tag id: %s", contentQuery)
	}
	if !reflect.DeepEqual(contentArgs, []interface{}{"artist", "alice"}) {
		t.Fatalf("content args = %#v", contentArgs)
	}

	locationQuery, locationArgs := BuildLocations(expr, nil)
	if strings.Contains(locationQuery, "DISTINCT") || strings.Contains(locationQuery, "JOIN tags") {
		t.Fatalf("exact location tag query should avoid redundant tag join/deduplication: %s", locationQuery)
	}
	if !strings.Contains(locationQuery, "FROM content_tags ct JOIN locations l") {
		t.Fatalf("exact location tag query should drive from content_tags: %s", locationQuery)
	}
	if !reflect.DeepEqual(locationArgs, []interface{}{"artist", "alice"}) {
		t.Fatalf("location args = %#v", locationArgs)
	}
}

func TestBuildExactTagPreservesLocationSemantics(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, statement := range []string{
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, key TEXT NOT NULL COLLATE NOCASE, value TEXT NOT NULL COLLATE NOCASE, UNIQUE(key, value))`,
		`CREATE TABLE content_tags (content_hash TEXT NOT NULL, tag_id INTEGER NOT NULL, PRIMARY KEY (content_hash, tag_id))`,
		`CREATE INDEX idx_content_tags_tag_id_content_hash ON content_tags(tag_id, content_hash)`,
		`CREATE TABLE locations (id INTEGER PRIMARY KEY, content_hash TEXT NOT NULL)`,
		`CREATE INDEX idx_locations_content_hash ON locations(content_hash)`,
		`INSERT INTO tags (id, key, value) VALUES (7, 'Artist', 'Alice'), (8, 'artist', 'Bob')`,
		`INSERT INTO content_tags (content_hash, tag_id) VALUES ('shared', 7), ('other', 8)`,
		`INSERT INTO locations (id, content_hash) VALUES (11, 'shared'), (12, 'shared'), (13, 'other')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	expr, err := Parse("artist:alice")
	if err != nil {
		t.Fatal(err)
	}
	query, args := BuildLocations(expr, nil)
	rows, err := db.Query(query+" ORDER BY id", args...)
	if err != nil {
		t.Fatalf("query %q with %v: %v", query, args, err)
	}
	defer rows.Close()

	var got []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []int64{11, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("location ids = %v, want %v", got, want)
	}

	planRows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer planRows.Close()
	var plan strings.Builder
	for planRows.Next() {
		var id, parent, unused int
		var detail string
		if err := planRows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteByte('\n')
	}
	if !strings.Contains(plan.String(), "idx_content_tags_tag_id_content_hash") {
		t.Fatalf("query plan should use covering exact-tag index; plan:\n%s", plan.String())
	}
}

func TestBuildTagWildcardKeepsGeneralSemantics(t *testing.T) {
	expr, err := Parse("artist:*")
	if err != nil {
		t.Fatal(err)
	}
	query, _ := BuildLocations(expr, nil)
	if !strings.Contains(query, "t.value != ''") || !strings.Contains(query, "JOIN tags") {
		t.Fatalf("wildcard tag query unexpectedly took exact-tag path: %s", query)
	}
}
