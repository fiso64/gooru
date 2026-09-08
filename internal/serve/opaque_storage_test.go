package serve

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOpaqueManagedStoragePathIsRandomAndExtensionless(t *testing.T) {
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
		if filepath.Dir(path) != dir {
			t.Fatalf("opaque path %q escaped managed directory %q", path, dir)
		}
		if filepath.Ext(path) != "" {
			t.Fatalf("opaque path %q leaked an extension", path)
		}
		base := filepath.Base(path)
		if len(base) != opaqueManagedNameBytes*2 {
			t.Fatalf("opaque basename length = %d, want %d", len(base), opaqueManagedNameBytes*2)
		}
		if strings.Contains(base, "human-readable") || strings.Contains(base, hash[:12]) {
			t.Fatalf("opaque basename %q leaked logical/content identity", base)
		}
	}
}
