package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestEditPathRenamesManagedLogicalLocation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	physicalPath := filepath.Join(t.TempDir(), "managed.bin")
	if err := os.WriteFile(physicalPath, []byte("managed contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, err := client.hasher.HashFile(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.hasher.FileMetadata(physicalPath)
	if err != nil {
		t.Fatal(err)
	}

	logicalRoot := t.TempDir()
	oldLogicalPath := filepath.Join(logicalRoot, "old", "upload.bin")
	newLogicalPath := filepath.Join(logicalRoot, "renamed", "upload.dat")
	if _, err := os.Stat(oldLogicalPath); !os.IsNotExist(err) {
		t.Fatalf("old logical path unexpectedly exists: %v", err)
	}
	if _, err := os.Stat(newLogicalPath); !os.IsNotExist(err) {
		t.Fatalf("new logical path unexpectedly exists: %v", err)
	}

	if _, err := client.store.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, hash, oldLogicalPath, info.Size, info.ModTime.Unix(), ".bin"); err != nil {
		t.Fatal(err)
	}
	original, err := client.store.GetFileInfoByPath(oldLogicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, original.ID, physicalPath); err != nil {
		t.Fatal(err)
	}

	if err := client.EditPath(oldLogicalPath, newLogicalPath); err != nil {
		t.Fatalf("edit managed logical path: %v", err)
	}

	updated, err := client.store.GetFileInfoByPath(newLogicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != original.ID {
		t.Fatalf("location id changed from %d to %d", original.ID, updated.ID)
	}
	if updated.Hash != hash {
		t.Fatalf("content hash changed from %q to %q", hash, updated.Hash)
	}
	if updated.Size != info.Size || updated.ModTime != info.ModTime.Unix() {
		t.Fatalf("metadata = size %d modtime %d, want size %d modtime %d", updated.Size, updated.ModTime, info.Size, info.ModTime.Unix())
	}
	if updated.Extension != ".dat" {
		t.Fatalf("extension = %q, want .dat", updated.Extension)
	}

	var gotPhysical string
	if err := client.store.QueryRow(`SELECT physical_path FROM managed_storage_locations WHERE location_id = ?`, original.ID).Scan(&gotPhysical); err != nil {
		t.Fatal(err)
	}
	if gotPhysical != physicalPath {
		t.Fatalf("physical path = %q, want %q", gotPhysical, physicalPath)
	}
}
