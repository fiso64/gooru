package scanning

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/hashing"
	"gooru.local/types"
)

func TestDirsConcurrentlyPrunesDuplicateAndNestedRoots(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	content := []byte("known-size-content")
	path := filepath.Join(nested, "sample.bin")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write sample file: %v", err)
	}

	hasher, err := hashing.NewHasher(types.StrategyFull)
	if err != nil {
		t.Fatalf("create hasher: %v", err)
	}

	locations, filesScanned, err := DirsConcurrently(
		[]string{root, root, nested},
		map[int64][]string{int64(len(content)): {"known-hash"}},
		hasher,
	)
	if err != nil {
		t.Fatalf("scan directories: %v", err)
	}
	if filesScanned != 1 {
		t.Fatalf("files scanned = %d, want 1", filesScanned)
	}
	if len(locations) != 1 {
		t.Fatalf("locations = %d, want 1", len(locations))
	}
	if _, ok := locations[path]; !ok {
		t.Fatalf("expected scanned location %q, got %#v", path, locations)
	}
}
