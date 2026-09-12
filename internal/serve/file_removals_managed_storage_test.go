package serve

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestManagedRemovalResolvesPhysicalStoragePath(t *testing.T) {
	managedRoot := t.TempDir()
	physicalPath := filepath.Join(managedRoot, "stored.jpg")
	if err := os.WriteFile(physicalPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	logicalPath := filepath.Join(t.TempDir(), "logical", "stored.jpg")
	file := types.FileInfo{PublicID: "managed-file", Path: logicalPath, StoragePath: physicalPath}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: managedRoot}}
	server := NewServerWithLibrary(cfg, emptyLibrary{})

	if server.canDeleteFilePath(file.Path) {
		t.Fatalf("logical path outside upload target must not pass managed deletion policy: %q", file.Path)
	}
	if !server.canDeleteFilePath(fileStoragePath(file)) {
		t.Fatalf("physical managed storage path should pass deletion policy: %q", physicalPath)
	}

	task, err := server.backgroundFileRemovalBatchTask("delete", []types.FileInfo{file})
	if err != nil {
		t.Fatal(err)
	}
	var input backgroundFileRemovalBatchInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		t.Fatal(err)
	}
	if len(input.Files) != 1 || input.Files[0].OriginalPath != physicalPath {
		t.Fatalf("managed delete task used wrong backing path: %+v", input.Files)
	}
}
