package query

import (
	"database/sql"
	"strings"
	"testing"

	_ "gosqlite.org"
)

func TestBuildLocationsMediaTypeUsesSelectiveIndexes(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, statement := range []string{
		`CREATE TABLE locations (id INTEGER PRIMARY KEY, content_hash TEXT NOT NULL, extension TEXT NOT NULL)`,
		`CREATE INDEX idx_locations_extension_lower ON locations(lower(extension))`,
		`CREATE TABLE media_metadata (content_hash TEXT PRIMARY KEY, media_kind TEXT NOT NULL)`,
		`CREATE INDEX idx_media_metadata_kind_lower_content ON media_metadata(lower(media_kind), content_hash)`,
		`INSERT INTO locations (id, content_hash, extension) VALUES (1, 'metadata-photo', '.bin'), (2, 'fallback-photo', '.jpg'), (3, 'overridden', '.jpg'), (4, 'video', '.mp4')`,
		`INSERT INTO media_metadata (content_hash, media_kind) VALUES ('metadata-photo', 'PHOTO'), ('overridden', 'audio')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	expr, err := Parse("type:photo")
	if err != nil {
		t.Fatal(err)
	}
	query, args := BuildLocations(expr, nil)

	rows, err := db.Query(query+" ORDER BY id", args...)
	if err != nil {
		t.Fatalf("query %q with %v: %v", query, args, err)
	}
	var got []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("photo ids = %v, want [1 2]", got)
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
	gotPlan := plan.String()
	if !strings.Contains(gotPlan, "idx_media_metadata_kind_lower_content") {
		t.Fatalf("media-backed branch should use media-kind expression index; plan:\n%s", gotPlan)
	}
	if !strings.Contains(gotPlan, "idx_locations_extension_lower") {
		t.Fatalf("fallback branch should use extension expression index; plan:\n%s", gotPlan)
	}
}

func TestBuildMediaTypeUnknownKindUsesMetadataWithCBZOverride(t *testing.T) {
	expr, err := Parse("type:audio")
	if err != nil {
		t.Fatal(err)
	}
	query, args := BuildLocations(expr, nil)
	if strings.Contains(query, "UNION") {
		t.Fatalf("metadata-only kind should not add an extension fallback branch: %s", query)
	}
	if !strings.Contains(query, "lower(l.extension) <> '.cbz'") {
		t.Fatalf("metadata-only kind must exclude authoritative CBZ comic rows: %s", query)
	}
	if len(args) != 1 || args[0] != "audio" {
		t.Fatalf("args = %#v", args)
	}
}
