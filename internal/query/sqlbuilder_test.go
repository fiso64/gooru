package query

import (
	"database/sql"
	"reflect"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestBuildLocationsMediaKind(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, statement := range []string{
		`CREATE TABLE locations (id INTEGER PRIMARY KEY, content_hash TEXT NOT NULL, path TEXT NOT NULL DEFAULT '', extension TEXT NOT NULL)`,
		`CREATE TABLE media_metadata (location_id INTEGER PRIMARY KEY, media_kind TEXT)`,
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, key TEXT NOT NULL, value TEXT NOT NULL)`,
		`CREATE TABLE content_tags (content_hash TEXT NOT NULL, tag_id INTEGER NOT NULL)`,
		`INSERT INTO locations (id, content_hash, extension) VALUES
			(1, 'photo', '.jpg'),
			(2, 'video', '.bin'),
			(3, 'gif', '.gif'),
			(4, 'metadata-wins', '.mp4'),
			(5, 'legacy-kind', '.txt')`,
		`INSERT INTO media_metadata (location_id, media_kind) VALUES
			(2, 'video'),
			(4, 'audio')`,
		`INSERT INTO tags (id, key, value) VALUES (1, 'kind', 'image')`,
		`INSERT INTO content_tags (content_hash, tag_id) VALUES ('legacy-kind', 1)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	for _, test := range []struct {
		query string
		want  []int64
	}{
		{query: "kind:photo", want: []int64{1}},
		{query: "kind:video", want: []int64{2}},
		{query: "kind:gif", want: []int64{3}},
		{query: "kind:audio", want: []int64{4}},
		{query: "kind:image", want: []int64{5}},
	} {
		t.Run(test.query, func(t *testing.T) {
			expr, err := Parse(test.query)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateAST(expr); err != nil {
				t.Fatal(err)
			}
			query, args := BuildLocations(expr, nil)
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
