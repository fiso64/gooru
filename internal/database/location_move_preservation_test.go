package database

import (
	"testing"

	"gooru.local/types"
)

func seedLocationScopedState(t *testing.T, store *Store, path string) int64 {
	t.Helper()
	const hash = "move-hash"
	if _, err := store.Exec(`INSERT OR IGNORE INTO contents (hash) VALUES (?)`, hash); err != nil {
		t.Fatal(err)
	}
	if err := store.GetOrCreateLocation(store.DB, hash, path, 10, 20, ".jpg"); err != nil {
		t.Fatal(err)
	}
	file, err := store.GetFileInfoByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertMediaMetadata(types.MediaMetadata{
		LocationID: file.ID,
		MediaKind:  "photo",
		MimeType:   "image/jpeg",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, file.ID, "/managed/object.bin"); err != nil {
		t.Fatal(err)
	}
	return file.ID
}

func assertLocationScopedState(t *testing.T, store *Store, path string, wantID int64) {
	t.Helper()
	file, err := store.GetFileInfoByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != wantID {
		t.Fatalf("location id changed from %d to %d", wantID, file.ID)
	}
	if file.Metadata == nil || file.Metadata.MediaKind != "photo" || file.Metadata.MimeType != "image/jpeg" {
		t.Fatalf("media metadata not preserved: %#v", file.Metadata)
	}
	var physicalPath string
	if err := store.QueryRow(`SELECT physical_path FROM managed_storage_locations WHERE location_id = ?`, wantID).Scan(&physicalPath); err != nil {
		t.Fatal(err)
	}
	if physicalPath != "/managed/object.bin" {
		t.Fatalf("physical path = %q, want managed mapping", physicalPath)
	}
}

func TestUpdatePathPreservesLocationScopedMetadata(t *testing.T) {
	store := newMemoryTestStore(t)
	const oldPath = "/library/old.jpg"
	const newPath = "/library/new.jpg"
	id := seedLocationScopedState(t, store, oldPath)

	if err := store.UpdatePath(oldPath, types.LocationInfo{
		Path:      newPath,
		Size:      30,
		ModTime:   40,
		Extension: ".jpg",
	}, oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	assertLocationScopedState(t, store, newPath, id)
}

func TestUpdateMovedLocationPreservesLocationScopedMetadata(t *testing.T) {
	store := newMemoryTestStore(t)
	const oldPath = "/library/old.jpg"
	const newPath = "/library/new.jpg"
	id := seedLocationScopedState(t, store, oldPath)

	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMovedLocation(tx, oldPath, types.LocationInfo{
		Path:      newPath,
		Size:      30,
		ModTime:   40,
		Extension: ".jpg",
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertLocationScopedState(t, store, newPath, id)
}
