package gooru

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gooru.local/types"
)

func TestTagFilesPersistsAddedAtInMilliseconds(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	path := filepath.Join(t.TempDir(), "external.jpg")
	if err := os.WriteFile(path, []byte("external content"), 0o600); err != nil {
		t.Fatalf("write external file: %v", err)
	}

	before := time.Now().Add(-time.Second).UnixMilli()
	if _, err := client.TagFiles([]string{path}, []string{"external"}, nil, false); err != nil {
		t.Fatalf("TagFiles: %v", err)
	}
	after := time.Now().Add(time.Second).UnixMilli()

	var addedAt int64
	if err := client.store.DB.QueryRow(`SELECT added_at FROM locations WHERE path = ?`, path).Scan(&addedAt); err != nil {
		t.Fatalf("read added_at: %v", err)
	}
	if addedAt < before || addedAt > after {
		t.Fatalf("added_at=%d, want current Unix milliseconds in [%d, %d]", addedAt, before, after)
	}
}

func TestNormalizeRegistrationAddedAtPreservesExplicitMilliseconds(t *testing.T) {
	const explicit = int64(31_536_000_000) // 1971-01-01 UTC in Unix milliseconds.
	path := "/library/early.jpg"
	locations := map[string]types.LocationInfo{
		path: {Path: path, AddedAt: explicit},
	}

	normalizeRegistrationAddedAt(locations)
	if got := locations[path].AddedAt; got != explicit {
		t.Fatalf("explicit added_at=%d changed, want %d", got, explicit)
	}
}
