package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanupPlaintextMediaCacheRemovesOnlyOwnedDerivatives(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media-cache")
	ownedName := strings.Repeat("a", 64) + ".jpg"
	owned := filepath.Join(root, "aa", ownedName)
	unrelated := filepath.Join(root, "keep.txt")
	unrelatedInShard := filepath.Join(root, "aa", "keep.txt")
	otherDirFile := filepath.Join(root, "custom", strings.Repeat("b", 64)+".png")
	for path, content := range map[string]string{
		owned:          "plaintext derivative",
		unrelated:      "unrelated",
		unrelatedInShard: "unrelated shard content",
		otherDirFile:   "not a gooru shard",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := CleanupPlaintextMediaCache(MediaConfig{CacheDir: root}); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatalf("owned plaintext derivative remains, stat error = %v", err)
	}
	for _, path := range []string{unrelated, unrelatedInShard, otherDirFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("cleanup touched unrelated path %q: %v", path, err)
		}
	}
}

func TestCleanupPlaintextMediaCachePrunesEmptyOwnedShard(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media-cache")
	shard := filepath.Join(root, "0f")
	path := filepath.Join(shard, strings.Repeat("c", 64)+".png")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("plaintext derivative"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CleanupPlaintextMediaCache(MediaConfig{CacheDir: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shard); !os.IsNotExist(err) {
		t.Fatalf("empty owned shard remains, stat error = %v", err)
	}
}

func TestCleanupPlaintextMediaCacheIgnoresMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if err := CleanupPlaintextMediaCache(MediaConfig{CacheDir: root}); err != nil {
		t.Fatalf("missing cache root should be a no-op: %v", err)
	}
}
