package database

import (
	"database/sql"
	"io"
	"log"
	"strings"
	"testing"
)

func TestCountAllFilesTracksKindSummary(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}
	insert := func(hash, publicID, path, extension string) int64 {
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

	photoID := insert("photo", "file_photo", "/photo.jpg", ".jpg")
	insert("video", "file_video", "/video.mp4", ".mp4")
	insert("other", "file_other", "/notes.txt", ".txt")

	assertCount := func(want int) {
		t.Helper()
		got, err := store.CountAllFiles()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("CountAllFiles() = %d, want %d", got, want)
		}
	}
	assertCount(3)

	if _, err := db.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES ('photo', 'video', 'video/mp4')`); err != nil {
		t.Fatal(err)
	}
	assertCount(3)

	if _, err := db.Exec(`DELETE FROM locations WHERE id = ?`, photoID); err != nil {
		t.Fatal(err)
	}
	assertCount(2)
}

func TestRootLibraryCountPlanDoesNotReadLocations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT COALESCE(SUM(files_count), 0) FROM kind_counts`)
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
	if strings.Contains(plan, "locations") || strings.Contains(plan, "media_metadata") {
		t.Fatalf("root library count still reads library-sized tables:\n%s", plan)
	}
	if !strings.Contains(plan, "kind_counts") {
		t.Fatalf("root library count does not read kind_counts:\n%s", plan)
	}
}
