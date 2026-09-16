package database

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestKindCountsMigrationBackfillsAndTracksMutations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureMigrationTable(db); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.version >= 12 {
			break
		}
		if err := applyMigration(db, migration); err != nil {
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
	}

	insertLocation := func(hash, publicID, path, extension string) int64 {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
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

	photoID := insertLocation("photo", "file_photo", "/photo.jpg", ".jpg")
	gifID := insertLocation("gif", "file_gif", "/anim.gif", ".gif")
	otherID := insertLocation("other", "file_other", "/notes.txt", ".txt")
	comicID := insertLocation("comic", "file_comic", "/book.cbz", ".cbz")
	videoID := insertLocation("video", "file_video", "/clip.mp4", ".mp4")
	if _, err := db.Exec(`
		INSERT INTO media_metadata (location_id, media_kind, mime_type)
		VALUES (?, 'photo', 'image/jpeg'), (?, 'other', 'application/vnd.comicbook+zip')
	`, videoID, comicID); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`UPDATE media_metadata SET media_kind = 'photo' WHERE content_hash = 'comic'`); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES ('photo', 'video', 'video/mp4')`); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`UPDATE media_metadata SET media_kind = 'gif' WHERE content_hash = 'photo'`); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`UPDATE locations SET extension = '.png' WHERE id = ?`, otherID); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`UPDATE locations SET extension = '.zip' WHERE id = ?`, comicID); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`UPDATE locations SET extension = '.cbz' WHERE id = ?`, comicID); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`DELETE FROM media_metadata WHERE content_hash = 'photo'`); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`DELETE FROM locations WHERE id = ?`, videoID); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`DELETE FROM locations WHERE id = ?`, gifID); err != nil {
		t.Fatal(err)
	}
	assertKindCountsMatchRecomputed(t, db)
}

func TestRootKindFacetSummaryPlanDoesNotReadLibraryTables(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT kind, files_count
		FROM kind_counts
		WHERE files_count > 0
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
	if strings.Contains(plan, "locations") || strings.Contains(plan, "media_metadata") {
		t.Fatalf("root kind facet summary still reads library-sized tables:\n%s", plan)
	}
	if !strings.Contains(plan, "kind_counts") {
		t.Fatalf("root kind facet summary does not read kind_counts:\n%s", plan)
	}
}

func assertKindCountsMatchRecomputed(t *testing.T, db *sql.DB) {
	t.Helper()
	readCounts := func(query string) map[string]int {
		t.Helper()
		rows, err := db.Query(query)
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

	want := readCounts(`
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
		GROUP BY kind
	`)
	got := readCounts(`SELECT kind, files_count FROM kind_counts WHERE files_count > 0`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kind_counts = %#v, recomputed = %#v", got, want)
	}
}
