package database

import (
	"database/sql"
	"testing"
)

func TestMediaMetadataInvalidatedWhenLocationContentChanges(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('old'), ('new')`); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES ('file_metadata', 'old', '/library/photo.jpg', 1, 1, '.jpg')
	`)
	if err != nil {
		t.Fatal(err)
	}
	locationID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_metadata (location_id, media_kind, mime_type, image_width, image_height)
		VALUES (?, 'image', 'image/jpeg', 640, 480)
	`, locationID); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`UPDATE locations SET size_bytes = 2 WHERE id = ?`, locationID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM media_metadata WHERE location_id = ?`, locationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("metadata rows after non-content update = %d, want 1", count)
	}

	if _, err := db.Exec(`UPDATE locations SET content_hash = 'new' WHERE id = ?`, locationID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM media_metadata WHERE location_id = ?`, locationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("metadata rows after content replacement = %d, want 0", count)
	}
}
