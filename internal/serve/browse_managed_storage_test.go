package serve

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestFileDTOCanDeleteManagedStoragePath(t *testing.T) {
	managedRoot := t.TempDir()
	physicalPath := filepath.Join(managedRoot, "stored.jpg")
	if err := os.WriteFile(physicalPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	logicalPath := filepath.Join(t.TempDir(), "logical", "stored.jpg")
	file := types.FileInfo{
		PublicID:    "managed-file",
		Path:        logicalPath,
		StoragePath: physicalPath,
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: managedRoot}}
	server := NewServerWithLibrary(cfg, emptyLibrary{})

	if server.canDeleteFilePath(file.Path) {
		t.Fatalf("logical path outside upload target must not pass managed deletion policy: %q", file.Path)
	}
	if dto := server.fileDTO(context.Background(), file, false); !dto.CanDelete {
		t.Fatalf("managed file backed by configured storage should be advertised as deletable: %+v", dto)
	}
}
