package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestRehashTrackedFilesReadsManagedPhysicalPath(t *testing.T) {
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
	if err := os.WriteFile(physicalPath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldHash, err := client.hasher.HashFile(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.hasher.FileMetadata(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	logicalPath := filepath.Join(t.TempDir(), "logical", "upload.bin") // deliberately absent on disk

	if _, err := client.store.Exec(`INSERT INTO contents (hash) VALUES (?)`, oldHash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, oldHash, logicalPath, info.Size, info.ModTime.Unix(), ".bin"); err != nil {
		t.Fatal(err)
	}
	file, err := client.store.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, file.ID, physicalPath); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(physicalPath, []byte("after content changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantHash, err := client.hasher.HashFile(physicalPath)
	if err != nil {
		t.Fatal(err)
	}

	var gotStatus types.RehashStatus
	var gotErr error
	client.RehashTrackedFiles([]string{logicalPath}, func(_ string, status types.RehashStatus, err error) {
		gotStatus = status
		gotErr = err
	}, false)
	if gotErr != nil {
		t.Fatalf("rehash managed logical path: %v", gotErr)
	}
	if gotStatus != types.StatusRehashed {
		t.Fatalf("status = %v, want StatusRehashed", gotStatus)
	}

	updated, err := client.store.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != file.ID {
		t.Fatalf("location id changed from %d to %d", file.ID, updated.ID)
	}
	if updated.Hash != wantHash {
		t.Fatalf("hash = %q, want %q", updated.Hash, wantHash)
	}
	var gotPhysical string
	if err := client.store.QueryRow(`SELECT physical_path FROM managed_storage_locations WHERE location_id = ?`, file.ID).Scan(&gotPhysical); err != nil {
		t.Fatal(err)
	}
	if gotPhysical != physicalPath {
		t.Fatalf("physical path = %q, want %q", gotPhysical, physicalPath)
	}
}

func TestRehashTrackedFilesRefreshesRepeatedPathAfterMutation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatal(err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	filePath := filepath.Join(t.TempDir(), "repeated.bin")
	if err := os.WriteFile(filePath, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldHash, err := client.hasher.HashFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	oldInfo, err := client.hasher.FileMetadata(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.store.Exec(`INSERT INTO contents (hash) VALUES (?)`, oldHash); err != nil {
		t.Fatal(err)
	}
	if err := client.store.GetOrCreateLocation(client.store, oldHash, filePath, oldInfo.Size, oldInfo.ModTime.Unix(), ".bin"); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filePath, []byte("after content changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantHash, err := client.hasher.HashFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	var statuses []types.RehashStatus
	var errs []error
	client.RehashTrackedFiles([]string{filePath, filePath}, func(_ string, status types.RehashStatus, err error) {
		statuses = append(statuses, status)
		errs = append(errs, err)
	}, false)

	if len(statuses) != 2 || len(errs) != 2 {
		t.Fatalf("callbacks = %d statuses/%d errors, want 2/2", len(statuses), len(errs))
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("callback %d error = %v", i, err)
		}
	}
	if statuses[0] != types.StatusRehashed {
		t.Fatalf("first status = %v, want StatusRehashed", statuses[0])
	}
	if statuses[1] != types.StatusSkippedUnchanged {
		t.Fatalf("second status = %v, want StatusSkippedUnchanged", statuses[1])
	}

	updated, err := client.store.GetFileInfoByPath(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Hash != wantHash {
		t.Fatalf("hash = %q, want %q", updated.Hash, wantHash)
	}
}
