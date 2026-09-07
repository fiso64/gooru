package database

import (
	"database/sql"
	"testing"
)

func TestTagKeyKindCountsTreatKeyCasingAsOneNamespace(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('case-test')`); err != nil {
		t.Fatal(err)
	}
	insertTagKindTestLocation(t, db, "case-test", "case-test", "/case.jpg", ".jpg")
	upper := insertTagKeyKindTestTag(t, db, "Artist", "alice")
	lower := insertTagKeyKindTestTag(t, db, "artist", "bob")
	for _, id := range []int64{upper, lower} {
		if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES ('case-test', ?)`, id); err != nil {
			t.Fatal(err)
		}
	}

	var rows, count int
	if err := db.QueryRow(`SELECT COUNT(*), coalesce(SUM(files_count), 0) FROM tag_key_kind_counts WHERE key = 'ARTIST' AND kind = 'photo'`).Scan(&rows, &count); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || count != 1 {
		t.Fatalf("case-insensitive key summary = rows %d, count %d; want one deduplicated row/count", rows, count)
	}
}
