package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestRehashFilesUsesManagedPhysicalSource(t *testing.T) {
	client := newManagedRelinkClient(t)
	logicalPath := filepath.Join(t.TempDir(), "library", "upload.txt") // deliberately absent on disk
	physicalPath := filepath.Join(t.TempDir(), "managed.txt")
	addManagedTestLocation(t, client, logicalPath, physicalPath, []byte("before"))

	if err := os.WriteFile(physicalPath, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}

	var gotStatus types.RehashStatus
	var gotErr error
	client.RehashFiles([]string{logicalPath}, func(path string, status types.RehashStatus, err error) {
		if path != logicalPath {
			t.Fatalf("callback path = %q, want %q", path, logicalPath)
		}
		gotStatus = status
		gotErr = err
	}, false)
	if gotErr != nil {
		t.Fatalf("RehashFiles managed path failed: %v", gotErr)
	}
	if gotStatus != types.StatusRehashed {
		t.Fatalf("status = %v, want %v", gotStatus, types.StatusRehashed)
	}

	tracked, err := client.store.GetLocationSourceByPath(logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	newHash, err := client.hasher.HashFile(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if tracked.Hash != newHash {
		t.Fatalf("tracked hash = %q, want %q", tracked.Hash, newHash)
	}
	if tracked.StoragePath != physicalPath {
		t.Fatalf("physical path = %q, want %q", tracked.StoragePath, physicalPath)
	}
}
