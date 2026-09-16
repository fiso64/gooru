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

func TestMediaMetadataSweepPaginatesPendingContent(t *testing.T) {
	store, db := newMediaMetadataSweepTestStore(t)
	defer db.Close()

	for i := 0; i < 130; i++ {
		hash := fmt.Sprintf("hash-%03d", i)
		path := fmt.Sprintf("/%03d.jpg", i)
		if _, err := db.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`
			INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
			VALUES (?, ?, ?, 1, 1, '.jpg')`, fmt.Sprintf("file_%03d", i), hash, path); err != nil {
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

func TestMediaMetadataSweepCoalescesSameHashLocations(t *testing.T) {
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
	if len(files) != 1 || files[0].Path != "/a.jpg" || files[0].Hash != "same" {
		t.Fatalf("pending content representatives = %#v, want only /a.jpg", files)
	}

	photo := types.MediaMetadata{MediaKind: "photo", MimeType: "image/jpeg"}
	wrote, err := store.UpsertMediaMetadataForLocation(files[0].ID, files[0].Hash, files[0].Path, photo)
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("expected representative metadata write")
	}
	files, err = store.ListPendingMediaMetadataFiles(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("pending locations after hash metadata write = %#v, want none", files)
	}

	var metadataRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata WHERE content_hash = 'same'`).Scan(&metadataRows); err != nil {
		t.Fatal(err)
	}
	if metadataRows != 1 {
		t.Fatalf("metadata rows for duplicate content = %d, want 1", metadataRows)
	}

	rows, err := db.Query(`SELECT id FROM locations WHERE content_hash = 'same' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("duplicate location ids = %#v, want 2", ids)
	}
	for _, id := range ids {
		meta, err := store.GetMediaMetadata(id)
		if err != nil {
			t.Fatalf("get metadata through location %d: %v", id, err)
		}
		if meta.MediaKind != "photo" || meta.MimeType != "image/jpeg" {
			t.Fatalf("location %d metadata = %#v, want shared photo metadata", id, meta)
		}
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
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata WHERE content_hash IN ('old', 'new')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale metadata rows = %d, want 0", count)
	}
}
