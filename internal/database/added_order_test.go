package database

import (
	"testing"

	"gooru.local/types"
)

func TestAddedOrderKeysetPaginationPreservesEqualTimestampOrder(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-a", "hash-b", "hash-c"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	const addedAt int64 = 1700000000123
	locations := map[string]types.LocationInfo{
		"/library/a.jpg": {Path: "/library/a.jpg", Hash: "hash-a", Size: 1, ModTime: 1, AddedAt: addedAt, Extension: ".jpg"},
		"/library/b.jpg": {Path: "/library/b.jpg", Hash: "hash-b", Size: 1, ModTime: 1, AddedAt: addedAt, Extension: ".jpg"},
		"/library/c.jpg": {Path: "/library/c.jpg", Hash: "hash-c", Size: 1, ModTime: 1, AddedAt: addedAt, Extension: ".jpg"},
	}
	orders := map[string]int64{
		"/library/a.jpg": 0,
		"/library/b.jpg": 1,
		"/library/c.jpg": 2,
	}
	if err := store.BatchUpsertLocationsWithAddedOrder(store.DB, locations, orders); err != nil {
		t.Fatalf("BatchUpsertLocationsWithAddedOrder: %v", err)
	}

	assertPagedPaths := func(order string, want []string) {
		t.Helper()
		first, err := store.GetAllFilesInfoPageSortedAddedOrder(2, nil, "added", order)
		if err != nil {
			t.Fatalf("first %s page: %v", order, err)
		}
		if len(first) != 2 {
			t.Fatalf("first %s page len=%d want=2", order, len(first))
		}
		cursor := &types.PageCursor{Sort: "added", Order: order, ID: first[len(first)-1].ID}
		second, err := store.GetAllFilesInfoPageSortedAddedOrder(2, cursor, "added", order)
		if err != nil {
			t.Fatalf("second %s page: %v", order, err)
		}
		got := make([]string, 0, len(first)+len(second))
		for _, file := range append(first, second...) {
			got = append(got, file.Path)
		}
		if len(got) != len(want) {
			t.Fatalf("%s paged paths=%v want=%v", order, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s paged paths=%v want=%v", order, got, want)
			}
		}
	}

	assertPagedPaths("asc", []string{"/library/a.jpg", "/library/b.jpg", "/library/c.jpg"})
	assertPagedPaths("desc", []string{"/library/c.jpg", "/library/b.jpg", "/library/a.jpg"})
}

func TestAddedOrderUpsertPreservesEarlyEpochMilliseconds(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-early"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	// 1971-01-01 UTC in Unix milliseconds. This is deliberately below the old
	// magnitude heuristic that mistook early millisecond values for Unix seconds.
	const addedAt int64 = 31_536_000_000
	path := "/library/early.jpg"
	locations := map[string]types.LocationInfo{
		path: {Path: path, Hash: "hash-early", Size: 1, ModTime: 1, AddedAt: addedAt, Extension: ".jpg"},
	}
	if err := store.BatchUpsertLocationsWithAddedOrder(store.DB, locations, map[string]int64{path: 7}); err != nil {
		t.Fatalf("BatchUpsertLocationsWithAddedOrder: %v", err)
	}
	var got int64
	if err := store.QueryRow(`SELECT added_at FROM locations WHERE path = ?`, path).Scan(&got); err != nil {
		t.Fatalf("read stored added_at: %v", err)
	}
	if got != addedAt {
		t.Fatalf("added_at=%d want early-epoch milliseconds %d unchanged", got, addedAt)
	}
}
