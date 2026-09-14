package database

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"testing"

	"gooru.local/types"
)

func newMediaMetadataSweepTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		db.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	return &Store{DB: db, logger: log.New(io.Discard, "", 0)}, db
}

func TestMediaMetadataSweepPaginatesPendingLocations(t *testing.T) {
	store, db := newMediaMetadataSweepTestStore(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('same')`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 130; i++ {
		path := fmt.Sprintf("/%03d.jpg", i)
		if _, err := db.Exec(`
			INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
			VALUES (?, 'same', ?, 1, 1, '.jpg')`, fmt.Sprintf("file_%03d", i), path); err != nil {
			t.Fatal(err)
		}
	}

	first, err := store.ListPendingMediaMetadataFiles(0, 128)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 128 {
		t.Fatalf("first pending page length = %d, want 128", len(first))
	}
	second, err := store.ListPendingMediaMetadataFiles(first[len(first)-1].ID, 128)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 {
		t.Fatalf("second pending page length = %d, want 2", len(second))
	}
	if second[0].ID <= first[len(first)-1].ID {
		t.Fatalf("keyset cursor did not advance: first last=%d second first=%d", first[len(first)-1].ID, second[0].ID)
	}
}

func TestMediaMetadataSweepTreatsSameHashLocationsIndependently(t *testing.T) {
	store, db := newMediaMetadataSweepTestStore(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('same')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES
		('file_a', 'same', '/a.jpg', 1, 1, '.jpg'),
		('file_b', 'same', '/b.bin', 1, 1, '.bin')`); err != nil {
		t.Fatal(err)
	}

	files, err := store.ListPendingMediaMetadataFiles(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("pending locations = %d, want 2", len(files))
	}
	if files[0].Hash != files[1].Hash || files[0].Path == files[1].Path {
		t.Fatalf("unexpected same-hash locations: %#v", files)
	}

	photo := types.MediaMetadata{MediaKind: "photo", MimeType: "image/jpeg"}
	wrote, err := store.UpsertMediaMetadataForLocation(files[0].ID, files[0].Hash, files[0].Path, photo)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("expected first location metadata write")
	}
	files, err = store.ListPendingMediaMetadataFiles(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "/b.bin" {
		t.Fatalf("remaining pending locations = %#v, want only /b.bin", files)
	}
}

func TestMediaMetadataSweepRejectsStaleLocationWrite(t *testing.T) {
	store, db := newMediaMetadataSweepTestStore(t)
	defer db.Close()

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('old'), ('new')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES ('file_a', 'old', '/a.jpg', 1, 1, '.jpg')`); err != nil {
		t.Fatal(err)
	}
	files, err := store.ListPendingMediaMetadataFiles(0, 10)
	if err != nil || len(files) != 1 {
		t.Fatalf("pending files = %#v err=%v", files, err)
	}
	file := files[0]

	if _, err := db.Exec(`UPDATE locations SET content_hash = 'new' WHERE id = ?`, file.ID); err != nil {
		t.Fatal(err)
	}
	wrote, err := store.UpsertMediaMetadataForLocation(file.ID, file.Hash, file.Path, types.MediaMetadata{MediaKind: "photo", MimeType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("stale rehash write unexpectedly succeeded")
	}

	if _, err := db.Exec(`UPDATE locations SET content_hash = 'old', path = '/a.bin', extension = '.bin' WHERE id = ?`, file.ID); err != nil {
		t.Fatal(err)
	}
	wrote, err = store.UpsertMediaMetadataForLocation(file.ID, file.Hash, file.Path, types.MediaMetadata{MediaKind: "photo", MimeType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("stale rename write unexpectedly succeeded")
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata WHERE location_id = ?`, file.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale metadata rows = %d, want 0", count)
	}
}
