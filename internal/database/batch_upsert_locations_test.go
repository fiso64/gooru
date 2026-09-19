package database

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"gooru.local/types"
)

type managedStorageExecCountingQuerier struct {
	Querier
	managedExecs int
}

func (q *managedStorageExecCountingQuerier) Exec(query string, args ...interface{}) (sql.Result, error) {
	if strings.Contains(query, "INSERT INTO managed_storage_locations") {
		q.managedExecs++
	}
	return q.Querier.Exec(query, args...)
}

func TestBatchUpsertLocationsBatchesManagedStorageWrites(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-one", "hash-two", "hash-three"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	firstPath := "/library/first.jpg"
	if err := store.BatchUpsertLocations(store.DB, map[string]types.LocationInfo{
		firstPath: {
			Path:        firstPath,
			Hash:        "hash-one",
			Size:        1,
			ModTime:     10,
			Extension:   ".jpg",
			StoragePath: "/managed/first-old.jpg",
		},
	}); err != nil {
		t.Fatalf("seed managed location: %v", err)
	}

	secondPath := "/library/second.jpg"
	externalPath := "/external/third.jpg"
	counting := &managedStorageExecCountingQuerier{Querier: store.DB}
	if err := store.BatchUpsertLocations(counting, map[string]types.LocationInfo{
		firstPath: {
			Path:        firstPath,
			Hash:        "hash-one",
			Size:        11,
			ModTime:     110,
			Extension:   ".jpg",
			StoragePath: "/managed/first-new.jpg",
		},
		secondPath: {
			Path:        secondPath,
			Hash:        "hash-two",
			Size:        2,
			ModTime:     20,
			Extension:   ".jpg",
			StoragePath: "/managed/second.jpg",
		},
		externalPath: {
			Path:        externalPath,
			Hash:        "hash-three",
			Size:        3,
			ModTime:     30,
			Extension:   ".jpg",
			StoragePath: "   ",
		},
	}); err != nil {
		t.Fatalf("batch upsert locations: %v", err)
	}
	if counting.managedExecs != 1 {
		t.Fatalf("managed storage Exec calls = %d, want 1 per location batch", counting.managedExecs)
	}

	rows, err := store.Query(`
		SELECT l.path, m.physical_path
		FROM managed_storage_locations m
		JOIN locations l ON l.id = m.location_id
		ORDER BY l.path`)
	if err != nil {
		t.Fatalf("query managed mappings: %v", err)
	}
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var path, physicalPath string
		if err := rows.Scan(&path, &physicalPath); err != nil {
			t.Fatalf("scan managed mapping: %v", err)
		}
		got[path] = physicalPath
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate managed mappings: %v", err)
	}
	want := map[string]string{
		firstPath:  "/managed/first-new.jpg",
		secondPath: "/managed/second.jpg",
	}
	if len(got) != len(want) {
		t.Fatalf("managed mappings = %#v, want %#v", got, want)
	}
	for path, physicalPath := range want {
		if got[path] != physicalPath {
			t.Fatalf("managed mapping for %q = %q, want %q", path, got[path], physicalPath)
		}
	}
	if _, exists := got[externalPath]; exists {
		t.Fatalf("blank StoragePath unexpectedly created managed mapping for %q", externalPath)
	}
}

func TestBatchUpsertLocationsManagedStorageAcrossBindBoundary(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash"}); err != nil {
		t.Fatalf("seed content: %v", err)
	}

	const columns = 6 // content_hash, path, size_bytes, mod_time, added_at, extension
	const count = maxVars/columns + 1
	locations := make(map[string]types.LocationInfo, count)
	for i := 0; i < count; i++ {
		path := fmt.Sprintf("/library/file-%03d.jpg", i)
		locations[path] = types.LocationInfo{
			Path: path, Hash: "hash", StoragePath: fmt.Sprintf("/managed/file-%03d.jpg", i),
			Size: int64(i + 1), Extension: ".jpg",
		}
	}

	counting := &managedStorageExecCountingQuerier{Querier: store.DB}
	if err := store.BatchUpsertLocations(counting, locations); err != nil {
		t.Fatalf("batch upsert over bind boundary: %v", err)
	}
	if counting.managedExecs != 2 {
		t.Fatalf("managed storage Exec calls = %d, want 2 across bind boundary", counting.managedExecs)
	}

	var got int
	if err := store.QueryRow("SELECT COUNT(*) FROM managed_storage_locations").Scan(&got); err != nil {
		t.Fatalf("count managed mappings: %v", err)
	}
	if got != count {
		t.Fatalf("managed mappings = %d, want %d", got, count)
	}
}
