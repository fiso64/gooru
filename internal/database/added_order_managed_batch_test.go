package database

import (
	"testing"

	"gooru.local/types"
)

func TestAddedOrderUpsertBatchesManagedStorageWrites(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-managed", "hash-external"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}

	managedPath := "/library/managed.jpg"
	externalPath := "/library/external.jpg"
	counting := &managedStorageExecCountingQuerier{Querier: store.DB}
	locations := map[string]types.LocationInfo{
		managedPath: {
			Path: managedPath, Hash: "hash-managed", Size: 1, ModTime: 1,
			AddedAt: 1000, Extension: ".jpg", StoragePath: "/managed/managed.jpg",
		},
		externalPath: {
			Path: externalPath, Hash: "hash-external", Size: 2, ModTime: 2,
			AddedAt: 1001, Extension: ".jpg", StoragePath: "   ",
		},
	}
	orders := map[string]int64{managedPath: 0, externalPath: 1}
	if err := store.BatchUpsertLocationsWithAddedOrder(counting, locations, orders); err != nil {
		t.Fatalf("BatchUpsertLocationsWithAddedOrder: %v", err)
	}
	if counting.managedExecs != 1 {
		t.Fatalf("managed storage Exec calls = %d, want 1 per location batch", counting.managedExecs)
	}
}
