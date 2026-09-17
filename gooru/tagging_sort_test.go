package gooru

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type storedRegistrationSort struct {
	addedAt    int64
	addedOrder int64
}

func readStoredRegistrationSort(t *testing.T, client *Client, path string) storedRegistrationSort {
	t.Helper()
	var got storedRegistrationSort
	if err := client.store.DB.QueryRow(`SELECT added_at, added_order FROM locations WHERE path = ?`, path).Scan(&got.addedAt, &got.addedOrder); err != nil {
		t.Fatalf("read registration sort for %q: %v", path, err)
	}
	return got
}

func writeRegistrationSortFiles(t *testing.T, dir string, names ...string) []string {
	t.Helper()
	paths := make([]string, 0, len(names))
	for index, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte{byte(index + 1)}, 0o600); err != nil {
			t.Fatalf("write %q: %v", path, err)
		}
		paths = append(paths, path)
	}
	return paths
}

func TestTagFilesWithSortQueueAndReverseQueue(t *testing.T) {
	for _, tc := range []struct {
		name string
		sort FileRegistrationSort
		want []int64
	}{
		{name: "queue", sort: FileRegistrationSortQueue, want: []int64{0, 1, 2}},
		{name: "reverse", sort: FileRegistrationSortReverseQueue, want: []int64{2, 1, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newBackgroundEnqueueTestClient(t)
			paths := writeRegistrationSortFiles(t, t.TempDir(), "a.jpg", "b.jpg", "c.jpg")
			if _, err := client.TagFilesWithSort(paths, nil, nil, false, tc.sort); err != nil {
				t.Fatalf("TagFilesWithSort: %v", err)
			}
			first := readStoredRegistrationSort(t, client, paths[0])
			for index, path := range paths {
				got := readStoredRegistrationSort(t, client, path)
				if got.addedAt != first.addedAt {
					t.Fatalf("%s added_at=%d want shared batch timestamp %d", path, got.addedAt, first.addedAt)
				}
				if got.addedOrder != tc.want[index] {
					t.Fatalf("%s added_order=%d want %d", path, got.addedOrder, tc.want[index])
				}
			}
		})
	}
}

func TestTagFilesWithSortModTime(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	paths := writeRegistrationSortFiles(t, t.TempDir(), "older.jpg", "newer.jpg")
	modtimes := []time.Time{
		time.Date(2020, 1, 2, 3, 4, 5, 123_000_000, time.UTC),
		time.Date(2021, 2, 3, 4, 5, 6, 456_000_000, time.UTC),
	}
	for index, path := range paths {
		if err := os.Chtimes(path, modtimes[index], modtimes[index]); err != nil {
			t.Fatalf("chtimes %q: %v", path, err)
		}
	}
	if _, err := client.TagFilesWithSort(paths, nil, nil, false, FileRegistrationSortModTime); err != nil {
		t.Fatalf("TagFilesWithSort: %v", err)
	}
	for index, path := range paths {
		got := readStoredRegistrationSort(t, client, path)
		if got.addedAt != modtimes[index].UnixMilli() {
			t.Fatalf("%s added_at=%d want modtime %d", path, got.addedAt, modtimes[index].UnixMilli())
		}
		if got.addedOrder != 0 {
			t.Fatalf("%s added_order=%d want 0", path, got.addedOrder)
		}
	}
}

func TestTagFilesWithSortPreservesExistingRegistrationOrder(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	path := writeRegistrationSortFiles(t, t.TempDir(), "existing.jpg")[0]
	if _, err := client.TagFilesWithSort([]string{path}, nil, nil, false, FileRegistrationSortQueue); err != nil {
		t.Fatalf("initial TagFilesWithSort: %v", err)
	}
	before := readStoredRegistrationSort(t, client, path)

	if _, err := client.TagFilesWithSort([]string{path}, []string{"again"}, nil, false, FileRegistrationSortModTime); err != nil {
		t.Fatalf("second TagFilesWithSort: %v", err)
	}
	after := readStoredRegistrationSort(t, client, path)
	if after != before {
		t.Fatalf("existing registration ordering changed: before=%+v after=%+v", before, after)
	}
}

func TestTagFilesWithSortRejectsUnknownStrategy(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	path := writeRegistrationSortFiles(t, t.TempDir(), "a.jpg")[0]
	if _, err := client.TagFilesWithSort([]string{path}, nil, nil, false, FileRegistrationSort("random")); err == nil {
		t.Fatal("expected invalid registration sort error")
	}
}
