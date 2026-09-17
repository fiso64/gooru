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
