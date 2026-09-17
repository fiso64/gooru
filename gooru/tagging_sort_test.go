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

func TestParseFileRegistrationSortNamesAndLegacyAliases(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  FileRegistrationSort
	}{
		{"newest-last", FileRegistrationSortNewestLast},
		{"newest_last", FileRegistrationSortNewestLast},
		{"queue", FileRegistrationSortNewestLast},
		{"newest-first", FileRegistrationSortNewestFirst},
		{"newest_first", FileRegistrationSortNewestFirst},
		{"reverse_queue", FileRegistrationSortNewestFirst},
		{"modtime", FileRegistrationSortModTime},
		{"mtime", FileRegistrationSortModTime},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseFileRegistrationSort(tc.input)
			if err != nil {
				t.Fatalf("ParseFileRegistrationSort(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ParseFileRegistrationSort(%q)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestResolveFileRegistrationSortMatchesUploadSemantics(t *testing.T) {
	batch := time.Date(2024, 1, 2, 3, 4, 5, 400_000_000, time.UTC)
	source := time.Date(2020, 7, 8, 9, 10, 11, 456_000_000, time.UTC)
	for _, tc := range []struct {
		name      string
		sort      FileRegistrationSort
		index     int
		wantTime  time.Time
		wantOrder int64
	}{
		{"newest last", FileRegistrationSortNewestLast, 1, batch, 1},
		{"newest first", FileRegistrationSortNewestFirst, 1, batch, 1},
		{"modtime keeps positional tie break", FileRegistrationSortModTime, 1, source, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFileRegistrationSort(tc.sort, source, batch, tc.index, 3)
			if !got.AddedAt.Equal(tc.wantTime) || got.AddedOrder != tc.wantOrder {
				t.Fatalf("ResolveFileRegistrationSort()=%+v want time=%v order=%d", got, tc.wantTime, tc.wantOrder)
			}
		})
	}
	first := ResolveFileRegistrationSort(FileRegistrationSortNewestFirst, time.Time{}, batch, 0, 3)
	if first.AddedOrder != 2 {
		t.Fatalf("newest-first first input added_order=%d want 2", first.AddedOrder)
	}
}

func TestTagFilesWithSortNewestLastAndNewestFirst(t *testing.T) {
	for _, tc := range []struct {
		name string
		sort FileRegistrationSort
		want []int64
	}{
		{name: "newest-last", sort: FileRegistrationSortNewestLast, want: []int64{0, 1, 2}},
		{name: "newest-first", sort: FileRegistrationSortNewestFirst, want: []int64{2, 1, 0}},
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
		if got.addedOrder != int64(index) {
			t.Fatalf("%s added_order=%d want positional tie-break %d", path, got.addedOrder, index)
		}
	}
}

func TestTagFilesWithSortPreservesExistingRegistrationOrder(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	path := writeRegistrationSortFiles(t, t.TempDir(), "existing.jpg")[0]
	if _, err := client.TagFilesWithSort([]string{path}, nil, nil, false, FileRegistrationSortNewestLast); err != nil {
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
