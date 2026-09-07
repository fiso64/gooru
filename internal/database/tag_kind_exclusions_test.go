package database

import (
	"path/filepath"
	"testing"
)

func TestKindFacetsForTagExclusionsDeduplicatesOverlappingTags(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "exclusions.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RunMigrations(store.DB); err != nil {
		t.Fatal(err)
	}

	for _, hash := range []string{"a", "b", "c"} {
		if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
	}
	hidden, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('Hidden', '')`)
	if err != nil {
		t.Fatal(err)
	}
	hiddenID, _ := hidden.LastInsertId()
	private, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('private', 'yes')`)
	if err != nil {
		t.Fatal(err)
	}
	privateID, _ := private.LastInsertId()

	for _, pair := range []struct {
		hash string
		tag  int64
	}{{"a", hiddenID}, {"a", privateID}, {"b", hiddenID}, {"c", privateID}} {
		if _, err := store.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`, pair.hash, pair.tag); err != nil {
			t.Fatal(err)
		}
	}
	for _, loc := range []struct {
		hash string
		id   string
		path string
		ext  string
	}{{"a", "file_a", "/a.jpg", ".jpg"}, {"b", "file_b", "/b.mp4", ".mp4"}, {"c", "file_c", "/c.cbz", ".cbz"}} {
		if _, err := store.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, 1, 1, ?)`, loc.id, loc.hash, loc.path, loc.ext); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.KindFacetsForTagExclusions([]TagFacetExclusion{
		{Key: "hidden", KeyOnly: true},
		{Key: "private", Value: "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}

	counts := make(map[string]int, len(got))
	for _, facet := range got {
		counts[facet.Tag] = facet.Count
	}
	want := map[string]int{"photo": 1, "video": 1, "comic": 1}
	if len(counts) != len(want) {
		t.Fatalf("facets = %#v, want %#v", counts, want)
	}
	for kind, count := range want {
		if counts[kind] != count {
			t.Fatalf("facets[%q] = %d, want %d (all facets %#v)", kind, counts[kind], count, counts)
		}
	}
}
