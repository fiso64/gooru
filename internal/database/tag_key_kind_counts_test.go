package database

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestTagKeyKindCountsDeduplicateSameKeyValuesAndTrackMutations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	for _, hash := range []string{"a", "b"} {
		if _, err := db.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
	}
	alice := insertTagKeyKindTestTag(t, db, "artist", "alice")
	bob := insertTagKeyKindTestTag(t, db, "artist", "bob")

	jpgID := insertTagKindTestLocation(t, db, "a", "a-jpg", "/a.jpg", ".jpg")
	if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES ('a', ?)`, alice); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")

	// A second value in the same namespace must not count the location twice.
	if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES ('a', ?)`, bob); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
	assertTagKeyKindCount(t, db, "artist", "photo", 1)

	cbzID := insertTagKindTestLocation(t, db, "b", "b-cbz", "/b.cbz", ".cbz")
	if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES ('b', ?)`, bob); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")

	// Removing one of multiple same-key values keeps the key-wide membership.
	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'a' AND tag_id = ?`, alice); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
	assertTagKeyKindCount(t, db, "artist", "photo", 1)

	if _, err := db.Exec(`UPDATE locations SET extension = '.jpg' WHERE id = ?`, cbzID); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
	if _, err := db.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES ('b', 'video', 'video/mp4')`); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
	if _, err := db.Exec(`UPDATE media_metadata SET media_kind = 'audio' WHERE content_hash = 'b'`); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
	if _, err := db.Exec(`DELETE FROM media_metadata WHERE content_hash = 'b'`); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")

	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'a' AND tag_id = ?`, bob); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
	if _, err := db.Exec(`DELETE FROM locations WHERE id = ?`, jpgID); err != nil {
		t.Fatal(err)
	}
	assertTagKeyKindCountsMatchRecomputed(t, db, "artist")
}

func TestTagKeyKindFacetSummaryPlanDoesNotReadLibraryTables(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT kind, files_count FROM tag_key_kind_counts
		WHERE key = 'hidden' AND files_count > 0
		ORDER BY files_count DESC, kind ASC`)
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
	plan := strings.ToLower(strings.Join(details, "\n"))
	for _, table := range []string{"locations", "media_metadata", "content_tags", "tag_kind_counts"} {
		if strings.Contains(plan, table) {
			t.Fatalf("key-wide kind summary still reads %s:\n%s", table, plan)
		}
	}
	if !strings.Contains(plan, "tag_key_kind_counts") {
		t.Fatalf("key-wide kind summary does not read tag_key_kind_counts:\n%s", plan)
	}
}

func insertTagKeyKindTestTag(t *testing.T, db *sql.DB, key, value string) int64 {
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

func assertTagKeyKindCount(t *testing.T, db *sql.DB, key, kind string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT coalesce(files_count, 0) FROM tag_key_kind_counts WHERE key = ? AND kind = ?`, key, kind).Scan(&got); err != nil {
		if err == sql.ErrNoRows && want == 0 {
			return
		}
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("tag_key_kind_counts[%q,%q] = %d, want %d", key, kind, got, want)
	}
}

func assertTagKeyKindCountsMatchRecomputed(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	read := func(query string, args ...interface{}) map[string]int {
		t.Helper()
		rows, err := db.Query(query, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		out := make(map[string]int)
		for rows.Next() {
			var kind string
			var count int
			if err := rows.Scan(&kind, &count); err != nil {
				t.Fatal(err)
			}
			if count > 0 {
				out[kind] = count
			}
		}
		return out
	}
	want := read(`
		SELECT CASE
			WHEN lower(l.extension) = '.cbz' THEN 'comic'
			ELSE coalesce(mm.media_kind, CASE
				WHEN lower(l.extension) = '.gif' THEN 'gif'
				WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
				WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
				ELSE 'other'
			END)
		END AS kind, COUNT(*)
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE EXISTS (
			SELECT 1 FROM content_tags ct JOIN tags t ON t.id = ct.tag_id
			WHERE ct.content_hash = l.content_hash AND t.key = ?
		)
		GROUP BY kind
	`, key)
	got := read(`SELECT kind, files_count FROM tag_key_kind_counts WHERE key = ? AND files_count > 0`, key)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tag_key_kind_counts = %#v, recomputed = %#v", got, want)
	}
}
