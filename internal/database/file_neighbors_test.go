package database

import (
	"fmt"
	"testing"

	"gooru.local/types"
)

func TestAnchoredNeighborsRespectCompleteSortTupleAndQuery(t *testing.T) {
	store := newMemoryTestStore(t)
	hashes := make([]string, 0, 12)
	locations := map[string]types.LocationInfo{}
	orderKeys := map[string]int64{}
	for i := 0; i < 12; i++ {
		hash := fmt.Sprintf("hash-%d", i)
		path := fmt.Sprintf("/library/%02d.jpg", i)
		hashes = append(hashes, hash)
		locations[path] = types.LocationInfo{Path: path, Hash: hash, Size: 10, ModTime: 1, AddedAt: 1000, Extension: ".jpg"}
		// Deliberately share the primary and secondary order keys in pairs.
		orderKeys[path] = int64(i / 4)
	}
	if err := store.BatchInsertContents(store.DB, hashes); err != nil {
		t.Fatal(err)
	}
	if err := store.BatchUpsertLocationsWithAddedOrder(store.DB, locations, orderKeys); err != nil {
		t.Fatal(err)
	}

	for _, sort := range []string{"added", "size", "modified", "name", "kind"} {
		for _, order := range []string{"asc", "desc"} {
			query := "SELECT id FROM locations WHERE CAST(substr(path, -6, 2) AS INTEGER) % 2 = 0"
			listed, err := store.GetFilesInfoByLocationQuerySorted(query, nil, sort, order)
			if err != nil {
				t.Fatal(err)
			}
			if len(listed) != 6 {
				t.Fatalf("%s/%s: filtered count %d", sort, order, len(listed))
			}
			for index, file := range listed {
				before, after, err := store.GetFilesInfoByLocationQueryAround(query, nil, file.ID, sort, order, 5)
				if err != nil {
					t.Fatalf("%s/%s anchor %d: %v", sort, order, file.ID, err)
				}
				for distance := 1; distance <= 5; distance++ {
					previous := listed[(index-distance+len(listed))%len(listed)].ID
					next := listed[(index+distance)%len(listed)].ID
					if before[distance-1].ID != previous || after[distance-1].ID != next {
						t.Fatalf("%s/%s anchor %d distance %d: before/after %d/%d want %d/%d", sort, order, file.ID, distance, before[distance-1].ID, after[distance-1].ID, previous, next)
					}
				}
			}
		}
	}
}
