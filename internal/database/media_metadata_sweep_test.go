package database

import (
	"database/sql"
	"io"
	"log"
	"testing"

	"gooru.local/types"
)

func TestMediaMetadataSweepReusesAndFansOutByContentHash(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}
	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('same'), ('other')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES
		('file_a', 'same', '/a.jpg', 1, 1, '.jpg'),
		('file_b', 'same', '/b.jpg', 1, 1, '.jpg'),
		('file_c', 'other', '/c.bin', 1, 1, '.bin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_metadata (location_id, media_kind, mime_type, image_width, image_height)
		SELECT id, 'photo', 'image/jpeg', 640, 480 FROM locations WHERE path = '/a.jpg'`); err != nil {
		t.Fatal(err)
	}

	hashes, err := store.ListPendingMediaMetadataContentHashes(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 2 || hashes[0] != "same" || hashes[1] != "other" {
		t.Fatalf("pending hashes = %#v, want [same other]", hashes)
	}
	cached, found, err := store.GetMediaMetadataByContentHash("same")
	if err != nil || !found {
		t.Fatalf("cached metadata found=%v err=%v", found, err)
	}
	if cached.ImageWidth == nil || *cached.ImageWidth != 640 {
		t.Fatalf("cached image width = %v, want 640", cached.ImageWidth)
	}
	if err := store.UpsertMediaMetadataForContentHash("same", cached); err != nil {
		t.Fatal(err)
	}
	var sameRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata mm JOIN locations l ON l.id = mm.location_id WHERE l.content_hash = 'same'`).Scan(&sameRows); err != nil {
		t.Fatal(err)
	}
	if sameRows != 2 {
		t.Fatalf("same-hash metadata rows = %d, want 2", sameRows)
	}

	if err := store.UpsertMediaMetadataForContentHash("other", types.MediaMetadata{MediaKind: "other", MimeType: "application/octet-stream"}); err != nil {
		t.Fatal(err)
	}
	hashes, err = store.ListPendingMediaMetadataContentHashes(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 0 {
		t.Fatalf("pending hashes after unsupported marker = %#v, want none", hashes)
	}
}
