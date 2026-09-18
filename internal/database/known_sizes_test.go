package database

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
)

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

func TestGetHashesBySizesRespectsBindBudget(t *testing.T) {
	store := newMemoryTestStore(t)
	sizes := make([]int64, maxVars+1)
	for i := range sizes {
		sizes[i] = int64(i + 1)
	}

	var queryLog bytes.Buffer
	store.logger = log.New(&queryLog, "", 0)
	hashes, err := store.GetHashesBySizes(sizes)
	if err != nil {
		t.Fatalf("GetHashesBySizes: %v", err)
	}
	if len(hashes) != 0 {
		t.Fatalf("hashes = %#v, want empty result", hashes)
	}

	logged := queryLog.String()
	if got := strings.Count(logged, "SELECT DISTINCT size_bytes, content_hash"); got != 2 {
		t.Fatalf("size-hash queries = %d, want 2\n%s", got, logged)
	}
	if !strings.Contains(logged, fmt.Sprintf("-- ARGS: %d bound values redacted", maxVars)) {
		t.Fatalf("missing max-sized batch in query log:\n%s", logged)
	}
	if !strings.Contains(logged, "-- ARGS: 1 bound values redacted") {
		t.Fatalf("missing remainder batch in query log:\n%s", logged)
	}
}
