package database

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func TestGetFileInfosByPublicIDsChunksPreservesOrderAndManagedStorage(t *testing.T) {
	store := newMemoryTestStore(t)
	count := maxVars + 1
	ids := make([]string, 0, count)
	paths := make(map[string]string, count)

	tx, err := store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < count; i++ {
		hash := fmt.Sprintf("hash-%d", i)
		publicID := fmt.Sprintf("file_%d", i)
		path := fmt.Sprintf("/library/%d.jpg", i)
		if _, err := tx.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatalf("insert content %d: %v", i, err)
		}
		if _, err := tx.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, ?, ?, ?)`, publicID, hash, path, i+1, 1000+i, ".jpg"); err != nil {
			t.Fatalf("insert location %d: %v", i, err)
		}
		ids = append(ids, publicID)
		paths[publicID] = path
	}
	var managedLocationID int64
	if err := tx.QueryRow(`SELECT id FROM locations WHERE public_id = ?`, ids[count/2]).Scan(&managedLocationID); err != nil {
		t.Fatalf("resolve managed location: %v", err)
	}
	const managedPath = "/managed/encrypted-container"
	if _, err := tx.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, managedLocationID, managedPath); err != nil {
		t.Fatalf("insert managed storage mapping: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	requested := make([]string, len(ids))
	for i := range ids {
		requested[i] = ids[len(ids)-1-i]
	}
	files, err := store.GetFileInfosByPublicIDs(requested)
	if err != nil {
		t.Fatalf("GetFileInfosByPublicIDs: %v", err)
	}
	if len(files) != len(requested) {
		t.Fatalf("got %d files, want %d", len(files), len(requested))
	}
	for i, file := range files {
		wantID := requested[i]
		if file.PublicID != wantID || file.Path != paths[wantID] {
			t.Fatalf("file %d = public_id %q path %q, want %q %q", i, file.PublicID, file.Path, wantID, paths[wantID])
		}
		if wantID == ids[count/2] && file.StoragePath != managedPath {
			t.Fatalf("managed storage path = %q, want %q", file.StoragePath, managedPath)
		}
	}

	_, err = store.GetFileInfosByPublicIDs(append(requested, "file_missing"))
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing public ID error = %v, want sql.ErrNoRows", err)
	}
}
