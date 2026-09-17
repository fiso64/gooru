package database

import "testing"

func TestGetKnownSizesDeduplicatesLocations(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-one", "hash-two", "hash-three"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}
	for _, location := range []struct {
		hash string
		path string
		size int64
	}{
		{hash: "hash-one", path: "/library/one.jpg", size: 10},
		{hash: "hash-two", path: "/library/two.jpg", size: 10},
		{hash: "hash-three", path: "/library/three.jpg", size: 20},
	} {
		if err := store.GetOrCreateLocation(store.DB, location.hash, location.path, location.size, 1, ".jpg"); err != nil {
			t.Fatalf("GetOrCreateLocation(%q): %v", location.path, err)
		}
	}

	sizes, err := store.GetKnownSizes()
	if err != nil {
		t.Fatalf("GetKnownSizes: %v", err)
	}
	if len(sizes) != 2 {
		t.Fatalf("known sizes = %#v, want exactly 2 distinct sizes", sizes)
	}
	for _, want := range []int64{10, 20} {
		if _, ok := sizes[want]; !ok {
			t.Fatalf("known sizes = %#v, missing %d", sizes, want)
		}
	}
}
