package database

import (
	"testing"

	"gooru.local/types"
)

func TestRehashLocationPreservingTagsKeepsLocationScopedMetadata(t *testing.T) {
	store := newMemoryTestStore(t)
	const (
		oldHash      = "old-hash"
		newHash      = "new-hash"
		path         = "/library/photo.jpg"
		physicalPath = "/managed/objects/photo.bin"
	)

	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, oldHash); err != nil {
		t.Fatal(err)
	}
	if err := store.GetOrCreateLocation(store.DB, oldHash, path, 10, 20, ".jpg"); err != nil {
		t.Fatal(err)
	}
	file, err := store.GetFileInfoByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	originalID := file.ID

	if err := store.UpsertMediaMetadata(types.MediaMetadata{
		LocationID: originalID,
		MediaKind:  "photo",
		MimeType:   "image/jpeg",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, originalID, physicalPath); err != nil {
		t.Fatal(err)
	}

	if err := store.RehashLocationPreservingTags(oldHash, newHash, types.LocationInfo{
		Path:      path,
		Hash:      newHash,
		Size:      30,
		ModTime:   40,
		Extension: ".jpg",
	}); err != nil {
		t.Fatal(err)
	}

	file, err = store.GetFileInfoByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != originalID {
		t.Fatalf("location id changed from %d to %d", originalID, file.ID)
	}
	if file.Hash != newHash {
		t.Fatalf("content hash = %q, want %q", file.Hash, newHash)
	}
	if file.Metadata == nil || file.Metadata.MediaKind != "photo" || file.Metadata.MimeType != "image/jpeg" {
		t.Fatalf("media metadata not preserved: %#v", file.Metadata)
	}

	var gotPhysicalPath string
	if err := store.QueryRow(`SELECT physical_path FROM managed_storage_locations WHERE location_id = ?`, originalID).Scan(&gotPhysicalPath); err != nil {
		t.Fatal(err)
	}
	if gotPhysicalPath != physicalPath {
		t.Fatalf("physical path = %q, want %q", gotPhysicalPath, physicalPath)
	}
}
