package database

import (
	"database/sql"
	"testing"

	"gooru.local/types"
)

func TestRehashLocationPreservingTagsKeepsLocationIdentityButInvalidatesDerivedMetadata(t *testing.T) {
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
		LocationID: file.ID,
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
	if file.Metadata != nil {
		t.Fatalf("stale media metadata preserved after byte-changing rehash: %#v", file.Metadata)
	}

	var metadataHash string
	if err := store.QueryRow(`SELECT content_hash FROM media_metadata WHERE content_hash = ?`, newHash).Scan(&metadataHash); err != sql.ErrNoRows {
		t.Fatalf("unexpected metadata row for changed content: hash=%q err=%v", metadataHash, err)
	}

	var gotPhysicalPath string
	if err := store.QueryRow(`SELECT physical_path FROM managed_storage_locations WHERE location_id = ?`, originalID).Scan(&gotPhysicalPath); err != nil {
		t.Fatal(err)
	}
	if gotPhysicalPath != physicalPath {
		t.Fatalf("physical path = %q, want %q", gotPhysicalPath, physicalPath)
	}
}

func TestRehashLocationPreservingTagsRefreshesCacheWhenTargetAlreadyHasTags(t *testing.T) {
	store := newMemoryTestStore(t)
	const (
		oldHash    = "old-hash"
		newHash    = "new-hash"
		rehashPath = "/library/changed.jpg"
		targetPath = "/library/existing.jpg"
	)

	for _, hash := range []string{oldHash, newHash} {
		if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.GetOrCreateLocation(store.DB, oldHash, rehashPath, 10, 20, ".jpg"); err != nil {
		t.Fatal(err)
	}
	if err := store.GetOrCreateLocation(store.DB, newHash, targetPath, 30, 40, ".jpg"); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('source', 'old')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('target', 'new')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`
		INSERT INTO content_tags (content_hash, tag_id)
		SELECT ?, id FROM tags WHERE key = 'source' AND value = 'old'`, oldHash); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`
		INSERT INTO content_tags (content_hash, tag_id)
		SELECT ?, id FROM tags WHERE key = 'target' AND value = 'new'`, newHash); err != nil {
		t.Fatal(err)
	}

	if err := store.RehashLocationPreservingTags(oldHash, newHash, types.LocationInfo{
		Path:      rehashPath,
		Hash:      newHash,
		Size:      30,
		ModTime:   40,
		Extension: ".jpg",
	}); err != nil {
		t.Fatal(err)
	}

	var cache string
	if err := store.QueryRow(`SELECT tags_cache FROM locations WHERE path = ?`, rehashPath).Scan(&cache); err != nil {
		t.Fatal(err)
	}
	if cache != "source:old target:new" {
		t.Fatalf("rehash tags_cache = %q, want merged target tags", cache)
	}

	var targetCache string
	if err := store.QueryRow(`SELECT tags_cache FROM locations WHERE path = ?`, targetPath).Scan(&targetCache); err != nil {
		t.Fatal(err)
	}
	if targetCache != cache {
		t.Fatalf("existing target cache = %q, rehashed cache = %q", targetCache, cache)
	}
}
