package query

import (
	"database/sql"
	"reflect"
	"testing"

	_ "gosqlite.org"
)

func TestBuildLocationsMediaType(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, statement := range []string{
		`CREATE TABLE locations (id INTEGER PRIMARY KEY, content_hash TEXT NOT NULL, path TEXT NOT NULL DEFAULT '', extension TEXT NOT NULL)`,
		`CREATE TABLE media_metadata (content_hash TEXT PRIMARY KEY, media_kind TEXT)`,
		`INSERT INTO locations (id, content_hash, extension) VALUES
			(1, 'photo', '.jpg'),
			(2, 'video', '.bin'),
			(3, 'gif', '.gif'),
			(4, 'metadata-wins', '.mp4'),
			(5, 'other', '.txt'),
			(6, 'comic', '.cbz')`,
		`INSERT INTO media_metadata (content_hash, media_kind) VALUES
			('video', 'video'),
			('metadata-wins', 'audio'),
			('comic', 'other')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}

	for _, test := range []struct {
		query string
		want  []int64
	}{
		{query: "type:photo", want: []int64{1}},
		{query: "type:video", want: []int64{2}},
		{query: "type:gif", want: []int64{3}},
		{query: "type:comic", want: []int64{6}},
		{query: "type:audio", want: []int64{4}},
		{query: "type:other", want: []int64{5}},
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
