package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpaqueManagedStoragePathIsRandomShardedAndExtensionless(t *testing.T) {
	dir := t.TempDir()
	logical := filepath.Join(dir, "human-readable-name.jpg")
	hash := strings.Repeat("a", 64)
	key := []byte("test-key-that-must-not-shape-the-name")

	first, err := OpaqueManagedStoragePath(logical, hash, key)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpaqueManagedStoragePath(logical, hash, key)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("opaque managed storage names must be freshly random")
	}
	for _, path := range []string{first, second} {
		base := filepath.Base(path)
		if len(base) != opaqueManagedNameBytes*2 {
			t.Fatalf("opaque basename length = %d, want %d", len(base), opaqueManagedNameBytes*2)
		}
		wantDir := filepath.Join(dir, protectedManagedNamespace, base[:2])
		if filepath.Dir(path) != wantDir {
			t.Fatalf("opaque path %q not in sharded protected namespace %q", path, wantDir)
		}
		if filepath.Ext(path) != "" {
			t.Fatalf("opaque path %q leaked an extension", path)
		}
		if strings.Contains(base, "human-readable") || strings.Contains(base, hash[:12]) {
			t.Fatalf("opaque basename %q leaked logical/content identity", base)
		}
	}
}

func TestOpaqueManagedStoragePathForTargetsUsesTargetRootForNestedLogicalPath(t *testing.T) {
	root := t.TempDir()
	logical := filepath.Join(root, "nested", "albums", "secret.jpg")
	targets := []UploadTarget{{ID: "managed", Path: root}}

	physical, err := OpaqueManagedStoragePathForTargets(targets, logical, "hash", []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(physical)
	wantDir := filepath.Join(root, protectedManagedNamespace, base[:2])
	if filepath.Dir(physical) != wantDir {
		t.Fatalf("physical path = %q, want namespace under target root %q", physical, wantDir)
	}
	if !IsProtectedManagedStoragePath(targets, physical) {
		t.Fatalf("generated path %q not recognized as protected managed storage", physical)
	}
}

func TestCleanupProtectedManagedStorageOrphansKeepsReferencedAndRemovesStrandedFiles(t *testing.T) {
	root := t.TempDir()
	targets := []UploadTarget{{ID: "managed", Path: root}}
	logical := filepath.Join(root, "secret.jpg")

	keep, err := OpaqueManagedStoragePathForTargets(targets, logical, "hash", nil)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := OpaqueManagedStoragePathForTargets(targets, logical, "hash", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("referenced ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("stranded ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := CleanupProtectedManagedStorageOrphans(targets, []string{keep}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("referenced protected file was removed: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphaned protected file still exists: %v", err)
	}
}
