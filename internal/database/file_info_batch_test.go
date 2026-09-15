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

func TestGetFileInfosByPathsOmitsMissingAndIncludesManagedStorage(t *testing.T) {
	store := newMemoryTestStore(t)
	tx, err := store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	for i, path := range []string{"/library/a.jpg", "/library/b.jpg"} {
		hash := fmt.Sprintf("path-hash-%d", i)
		publicID := fmt.Sprintf("path_file_%d", i)
		if _, err := tx.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatalf("insert content %d: %v", i, err)
		}
		if _, err := tx.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, ?, ?, ?)`, publicID, hash, path, i+1, 1000+i, ".jpg"); err != nil {
			t.Fatalf("insert location %d: %v", i, err)
		}
	}
	var managedLocationID int64
	if err := tx.QueryRow(`SELECT id FROM locations WHERE path = ?`, "/library/b.jpg").Scan(&managedLocationID); err != nil {
		t.Fatalf("resolve managed location: %v", err)
	}
	const managedPath = "/managed/b.enc"
	if _, err := tx.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, managedLocationID, managedPath); err != nil {
		t.Fatalf("insert managed storage mapping: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	files, err := store.GetFileInfosByPaths([]string{"/library/b.jpg", "/missing.jpg", "/library/a.jpg"})
	if err != nil {
		t.Fatalf("GetFileInfosByPaths: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d mapped paths, want 2", len(files))
	}
	if file := files["/library/a.jpg"]; file.PublicID != "path_file_0" {
		t.Fatalf("a.jpg public id = %q, want path_file_0", file.PublicID)
	}
	if file := files["/library/b.jpg"]; file.PublicID != "path_file_1" || file.StoragePath != managedPath {
		t.Fatalf("b.jpg = public id %q storage %q, want path_file_1 %q", file.PublicID, file.StoragePath, managedPath)
	}
	if _, ok := files["/missing.jpg"]; ok {
		t.Fatal("missing path unexpectedly returned")
	}
}

func TestGetFileInfosByContentHashesUsesDeterministicTrackedLocationAndOmitsUntrackedContent(t *testing.T) {
	store := newMemoryTestStore(t)
	tx, err := store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO contents (hash) VALUES (?), (?)`, "shared-hash", "orphan-hash"); err != nil {
		t.Fatalf("insert contents: %v", err)
	}
	for _, row := range []struct {
		publicID string
		path     string
	}{
		{publicID: "file_z", path: "/library/z.jpg"},
		{publicID: "file_a", path: "/library/a.jpg"},
	} {
		if _, err := tx.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, ?, ?, ?)`, row.publicID, "shared-hash", row.path, 1, 1000, ".jpg"); err != nil {
			t.Fatalf("insert location %s: %v", row.publicID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	files, err := store.GetFileInfosByContentHashes([]string{"orphan-hash", "shared-hash", "missing-hash"})
	if err != nil {
		t.Fatalf("GetFileInfosByContentHashes: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d mapped hashes, want 1 tracked hash", len(files))
	}
	shared, ok := files["shared-hash"]
	if !ok {
		t.Fatal("shared hash missing")
	}
	if shared.PublicID != "file_a" || shared.Path != "/library/a.jpg" {
		t.Fatalf("shared hash resolved to public id %q path %q, want deterministic /library/a.jpg", shared.PublicID, shared.Path)
	}
	if _, ok := files["orphan-hash"]; ok {
		t.Fatal("content without a tracked location unexpectedly returned")
	}
	if _, ok := files["missing-hash"]; ok {
		t.Fatal("missing content unexpectedly returned")
	}
}
