package query

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	_ "gosqlite.org"
)

func TestBuildLocationsFilenameContainsUsesTrigramIndex(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, statement := range []string{
		`CREATE TABLE locations (id INTEGER PRIMARY KEY, content_hash TEXT NOT NULL, path TEXT NOT NULL)`,
		`CREATE VIRTUAL TABLE location_filenames USING fts5(filename, tokenize='trigram')`,
		`INSERT INTO locations (id, content_hash, path) VALUES
			(1, 'one', '/directory-does-not-match/Foo-Bar_100%.JPG'),
			(2, 'two', '/needle-in-directory/plain.png'),
			(3, 'three', '/other/xy-file.txt')`,
		`INSERT INTO location_filenames(rowid, filename) VALUES
			(1, 'Foo-Bar_100%.JPG'),
			(2, 'plain.png'),
			(3, 'xy-file.txt')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	for _, test := range []struct {
		name       string
		query      string
		want       []int64
		wantMatch  bool
	}{
		{name: "case insensitive trigram", query: `@filename_contains:bar`, want: []int64{1}, wantMatch: true},
		{name: "punctuation is literal", query: `"@filename_contains:oo-"`, want: []int64{1}, wantMatch: true},
		{name: "percent is literal", query: `"@filename_contains:100%"`, want: []int64{1}, wantMatch: true},
		{name: "directory is excluded", query: `@filename_contains:needle`, want: nil, wantMatch: true},
		{name: "short fallback", query: `@filename_contains:xy`, want: []int64{3}, wantMatch: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			expr, err := Parse(test.query)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateAST(expr); err != nil {
				t.Fatal(err)
			}
			query, args := BuildLocations(expr, nil)
			if got := strings.Contains(query, "MATCH ?"); got != test.wantMatch {
				t.Fatalf("query MATCH presence = %t, want %t; SQL: %s", got, test.wantMatch, query)
			}
			if strings.Contains(query, "WITH RECURSIVE") {
				t.Fatalf("interactive filename query still recursively splits paths: %s", query)
			}

			rows, err := db.Query(query, args...)
			if err != nil {
				t.Fatalf("query %q with %v: %v", query, args, err)
			}
			var got []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					t.Fatal(err)
				}
				got = append(got, id)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			rows.Close()
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got ids %v, want %v", got, test.want)
			}
		})
	}
}

func TestQuoteFTS5Phrase(t *testing.T) {
	if got, want := quoteFTS5Phrase(`foo"bar`), `"foo""bar"`; got != want {
		t.Fatalf("quoteFTS5Phrase = %q, want %q", got, want)
	}
}
