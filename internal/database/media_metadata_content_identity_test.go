package database

import (
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"gooru.local/types"
)

func TestMediaMetadataIsSharedByContentHash(t *testing.T) {
	store := newMemoryTestStore(t)
	const hash = "shared-content"
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, path, ext string }{
		{"file_a", "/library/a.jpg", ".jpg"},
		{"file_b", "/library/b.bin", ".bin"},
	} {
		if _, err := store.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, 1, 1, ?)`, row.id, hash, row.path, row.ext); err != nil {
			t.Fatal(err)
		}
	}
	a, err := store.GetFileInfoByPath("/library/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.GetFileInfoByPath("/library/b.bin")
	if err != nil {
		t.Fatal(err)
	}
	width := 640
	if err := store.UpsertMediaMetadata(types.MediaMetadata{LocationID: a.ID, MediaKind: "photo", MimeType: "image/jpeg", ImageWidth: &width}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetMediaMetadata(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LocationID != b.ID || got.MediaKind != "photo" || got.ImageWidth == nil || *got.ImageWidth != width {
		t.Fatalf("metadata through duplicate location = %#v", got)
	}
	var rows int
	if err := store.QueryRow(`SELECT COUNT(*) FROM media_metadata WHERE content_hash = ?`, hash).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("metadata rows for shared hash = %d, want 1", rows)
	}

	const changed = "changed-content"
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, changed); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`UPDATE locations SET content_hash = ? WHERE id = ?`, changed, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetMediaMetadata(b.ID); err != sql.ErrNoRows {
		t.Fatalf("changed content metadata error = %v, want sql.ErrNoRows", err)
	}
	stillShared, err := store.GetMediaMetadata(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stillShared.MediaKind != "photo" {
		t.Fatalf("unchanged duplicate metadata = %#v", stillShared)
	}
}

func TestMediaMetadataDuplicateHashSummariesTrackMutationsAndRehash(t *testing.T) {
	store := newMemoryTestStore(t)
	const oldHash = "shared-summary-content"
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, oldHash); err != nil {
		t.Fatal(err)
	}
	res, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('artist', 'alice')`)
	if err != nil {
		t.Fatal(err)
	}
	tagID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`, oldHash, tagID); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ id, path string }{
		{"summary_a", "/library/summary-a.bin"},
		{"summary_b", "/library/summary-b.bin"},
	} {
		if _, err := store.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, ?, ?, 1, 1, '.bin')`, row.id, oldHash, row.path); err != nil {
			t.Fatal(err)
		}
	}

	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"other": 2})
	if _, err := store.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES (?, 'photo', 'image/jpeg')`, oldHash); err != nil {
		t.Fatal(err)
	}
	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"photo": 2})
	if _, err := store.Exec(`UPDATE media_metadata SET media_kind = 'video', mime_type = 'video/mp4' WHERE content_hash = ?`, oldHash); err != nil {
		t.Fatal(err)
	}
	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"video": 2})
	if _, err := store.Exec(`DELETE FROM media_metadata WHERE content_hash = ?`, oldHash); err != nil {
		t.Fatal(err)
	}
	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"other": 2})
	if _, err := store.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES (?, 'photo', 'image/jpeg')`, oldHash); err != nil {
		t.Fatal(err)
	}

	const newHash = "changed-summary-content"
	if err := store.RehashLocationPreservingTags(oldHash, newHash, types.LocationInfo{
		Path:      "/library/summary-b.bin",
		Hash:      newHash,
		Size:      2,
		ModTime:   2,
		Extension: ".bin",
	}); err != nil {
		t.Fatal(err)
	}
	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"photo": 1, "other": 1})

	if _, err := store.Exec(`INSERT INTO media_metadata (content_hash, media_kind, mime_type) VALUES (?, 'video', 'video/mp4')`, newHash); err != nil {
		t.Fatal(err)
	}
	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"photo": 1, "video": 1})
	if _, err := store.Exec(`DELETE FROM media_metadata WHERE content_hash = ?`, oldHash); err != nil {
		t.Fatal(err)
	}
	assertContentIdentityKindSummaries(t, store.DB, tagID, "artist", map[string]int{"other": 1, "video": 1})
}

func assertContentIdentityKindSummaries(t *testing.T, db *sql.DB, tagID int64, key string, want map[string]int) {
	t.Helper()
	read := func(query string, args ...interface{}) map[string]int {
		t.Helper()
		rows, err := db.Query(query, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		got := make(map[string]int)
		for rows.Next() {
			var kind string
			var count int
			if err := rows.Scan(&kind, &count); err != nil {
				t.Fatal(err)
			}
			if count > 0 {
				got[kind] = count
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return got
	}

	checks := []struct {
		name  string
		query string
		args  []interface{}
	}{
		{name: "root", query: `SELECT kind, files_count FROM kind_counts WHERE files_count > 0`},
		{name: "tag", query: `SELECT kind, files_count FROM tag_kind_counts WHERE tag_id = ? AND files_count > 0`, args: []interface{}{tagID}},
		{name: "tag key", query: `SELECT kind, files_count FROM tag_key_kind_counts WHERE key = ? AND files_count > 0`, args: []interface{}{key}},
	}
	for _, check := range checks {
		if got := read(check.query, check.args...); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s kind summary = %#v, want %#v", check.name, got, want)
		}
	}
}

func TestMediaMetadataContentIdentityMigrationDeduplicatesLegacyLocations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureMigrationTable(db); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.version >= 38 {
			break
		}
		if err := applyMigration(db, migration); err != nil {
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('same')`); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i, path := range []string{"/old.jpg", "/new.jpg"} {
		res, err := db.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension) VALUES (?, 'same', ?, 1, 1, '.jpg')`, []string{"file_old", "file_new"}[i], path)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	if _, err := db.Exec(`INSERT INTO media_metadata (location_id, media_kind, mime_type, updated_at) VALUES (?, 'photo', 'image/old', '2026-01-01 00:00:00'), (?, 'video', 'video/new', '2026-02-01 00:00:00')`, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	var hash, kind, mime string
	if err := db.QueryRow(`SELECT content_hash, media_kind, mime_type FROM media_metadata`).Scan(&hash, &kind, &mime); err != nil {
		t.Fatal(err)
	}
	if hash != "same" || kind != "video" || mime != "video/new" {
		t.Fatalf("migrated metadata = (%q, %q, %q)", hash, kind, mime)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_metadata`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migrated metadata row count = %d, want 1", count)
	}
	for _, id := range ids {
		var got string
		if err := db.QueryRow(`SELECT mm.media_kind FROM locations l JOIN media_metadata mm ON mm.content_hash = l.content_hash WHERE l.id = ?`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != "video" {
			t.Fatalf("location %d metadata kind = %q", id, got)
		}
	}
}

func TestBatchUpsertMediaMetadataCrossesBindLimit(t *testing.T) {
	store := newMemoryTestStore(t)
	const columns = 11
	count := maxVars/columns + 1
	hashes := make([]string, count)
	paths := make([]string, count)
	locations := make(map[string]types.LocationInfo, count)
	for i := range count {
		hashes[i] = fmt.Sprintf("batch-metadata-%03d", i)
		paths[i] = fmt.Sprintf("/library/batch-metadata-%03d.jpg", i)
		locations[paths[i]] = types.LocationInfo{
			Path:      paths[i],
			Hash:      hashes[i],
			Size:      1,
			ModTime:   1,
			Extension: ".jpg",
		}
	}
	if err := store.BatchInsertContents(store, hashes); err != nil {
		t.Fatal(err)
	}
	if err := store.BatchUpsertLocations(store, locations); err != nil {
		t.Fatal(err)
	}
	filesByPath, err := store.GetFileInfosByPaths(paths)
	if err != nil {
		t.Fatal(err)
	}

	metadata := make([]types.MediaMetadata, 0, count)
	for i, path := range paths {
		file, ok := filesByPath[path]
		if !ok {
			t.Fatalf("missing file info for %q", path)
		}
		width := 100 + i
		metadata = append(metadata, types.MediaMetadata{
			LocationID: file.ID,
			MediaKind:  "photo",
			MimeType:   "image/jpeg",
			ImageWidth: &width,
		})
	}
	if err := store.BatchUpsertMediaMetadata(metadata); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := store.QueryRow("SELECT COUNT(*) FROM media_metadata").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != count {
		t.Fatalf("metadata rows = %d, want %d", rows, count)
	}
	last, err := store.GetMediaMetadata(filesByPath[paths[count-1]].ID)
	if err != nil {
		t.Fatal(err)
	}
	if last.ImageWidth == nil || *last.ImageWidth != 100+count-1 {
		t.Fatalf("last metadata = %#v", last)
	}
}

func TestBatchUpsertMediaMetadataLastDuplicateContentWins(t *testing.T) {
	store := newMemoryTestStore(t)
	const hash = "batch-metadata-shared"
	if _, err := store.Exec("INSERT INTO contents (hash) VALUES (?)", hash); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/library/batch-a.bin", "/library/batch-b.bin"} {
		if _, err := store.Exec(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
			VALUES ('file_' || lower(hex(randomblob(16))), ?, ?, 1, 1, '.bin')`, hash, path); err != nil {
			t.Fatal(err)
		}
	}
	files, err := store.GetFileInfosByPaths([]string{"/library/batch-a.bin", "/library/batch-b.bin"})
	if err != nil {
		t.Fatal(err)
	}
	firstWidth, lastWidth := 320, 1920
	if err := store.BatchUpsertMediaMetadata([]types.MediaMetadata{
		{LocationID: files["/library/batch-a.bin"].ID, MediaKind: "photo", MimeType: "image/jpeg", ImageWidth: &firstWidth},
		{LocationID: files["/library/batch-b.bin"].ID, MediaKind: "video", MimeType: "video/mp4", ImageWidth: &lastWidth},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetMediaMetadata(files["/library/batch-a.bin"].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MediaKind != "video" || got.MimeType != "video/mp4" || got.ImageWidth == nil || *got.ImageWidth != lastWidth {
		t.Fatalf("metadata after duplicate-content batch = %#v", got)
	}
}
