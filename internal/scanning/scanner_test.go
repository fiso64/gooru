package scanning

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/hashing"
	"gooru.local/types"
)


func TestPruneRedundantDirsKeepsIndependentRootsInOrder(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	sibling := t.TempDir()

	got := PruneRedundantDirs([]string{root, nested, root, sibling})
	want := []string{root, sibling}
	if len(got) != len(want) {
		t.Fatalf("pruned roots = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pruned roots = %v, want %v", got, want)
		}
	}
}

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
		map[int64]struct{}{int64(len(content)): {}},
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
