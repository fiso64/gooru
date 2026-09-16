package database

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestTagKindCountsTrackEffectiveKindsAndMutations(t *testing.T) {
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
	res, err := db.Exec(`INSERT INTO tags (key, value) VALUES ('hidden', '')`)
	if err != nil {
		t.Fatal(err)
	}
	tagID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	// Exercise both association-before-location and location-before-association.
	if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES ('a', ?)`, tagID); err != nil {
		t.Fatal(err)
	}
	jpgID := insertTagKindTestLocation(t, db, "a", "file_a_jpg", "/a.jpg", ".jpg")
	cbzID := insertTagKindTestLocation(t, db, "a", "file_a_cbz", "/a.cbz", ".cbz")
	insertTagKindTestLocation(t, db, "b", "file_b_txt", "/b.txt", ".txt")
	if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES ('b', ?)`, tagID); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	// Legacy metadata must not override CBZ's authoritative comic kind.
	if _, err := db.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES ('a', 'other', 'application/vnd.comicbook+zip')`); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	if _, err := db.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES ('b', 'video', 'video/mp4')`); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	if _, err := db.Exec(`UPDATE media_metadata SET media_kind = 'audio' WHERE content_hash = 'b'`); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	if _, err := db.Exec(`UPDATE locations SET extension = '.zip' WHERE id = ?`, cbzID); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)
	if _, err := db.Exec(`UPDATE locations SET extension = '.cbz' WHERE id = ?`, cbzID); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	if _, err := db.Exec(`DELETE FROM media_metadata WHERE content_hash = 'b'`); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	if _, err := db.Exec(`DELETE FROM locations WHERE id = ?`, jpgID); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)

	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'b' AND tag_id = ?`, tagID); err != nil {
		t.Fatal(err)
	}
	assertTagKindCountsMatchRecomputed(t, db, tagID)
}

func TestTagKindFacetSummaryPlanDoesNotReadLibraryTables(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT tk.kind, tk.files_count
		FROM tags t
		JOIN tag_kind_counts tk ON tk.tag_id = t.id
		WHERE t.key = 'hidden' AND t.value = '' AND tk.files_count > 0
		ORDER BY tk.files_count DESC, tk.kind ASC`)
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
	for _, libraryTable := range []string{"locations", "media_metadata", "content_tags"} {
		if strings.Contains(plan, libraryTable) {
			t.Fatalf("exact-tag kind summary still reads %s:\n%s", libraryTable, plan)
		}
	}
	if !strings.Contains(plan, "tag_kind_counts") {
		t.Fatalf("exact-tag kind summary does not read tag_kind_counts:\n%s", plan)
	}
}

func insertTagKindTestLocation(t *testing.T, db *sql.DB, hash, publicID, path, extension string) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES (?, ?, ?, 1, 1, ?)
	`, publicID, hash, path, extension)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertTagKindCountsMatchRecomputed(t *testing.T, db *sql.DB, tagID int64) {
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
		if err := rows.Err(); err != nil {
			t.Fatal(err)
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
		FROM content_tags ct
		JOIN locations l ON l.content_hash = ct.content_hash
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE ct.tag_id = ?
		GROUP BY kind
	`, tagID)
	got := read(`SELECT kind, files_count FROM tag_kind_counts WHERE tag_id = ? AND files_count > 0`, tagID)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tag_kind_counts = %#v, recomputed = %#v", got, want)
	}
}
