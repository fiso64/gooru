package database

import (
	"database/sql"
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
