package database

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
)

func TestRunMigrationsRepairsHistoricalMediaMetadataVersionCollision(t *testing.T) {
	for _, collidedVersion := range []int{38, 39} {
		t.Run(fmt.Sprintf("version_%d", collidedVersion), func(t *testing.T) {
			db, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()

			migrations, err := loadEmbeddedMigrations()
			if err != nil {
				t.Fatal(err)
			}
			if len(migrations) == 0 {
				t.Fatal("no embedded migrations")
			}
			latestVersion := migrations[len(migrations)-1].version
			if err := ensureMigrationTable(db); err != nil {
				t.Fatal(err)
			}

			var migration39 embeddedMigration
			for _, migration := range migrations {
				if migration.version < historicalMediaMetadataCollisionVersion {
					if err := applyMigration(db, migration); err != nil {
						t.Fatalf("apply migration %d: %v", migration.version, err)
					}
					continue
				}
				if migration.version == 39 {
					migration39 = migration
				}
			}
			if migration39.version == 0 {
				t.Fatal("migration 39 not found")
			}

			if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('with-meta'), ('missing-meta')`); err != nil {
				t.Fatal(err)
			}
			res, err := db.Exec(`
				INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
				VALUES ('file_with_meta', 'with-meta', '/library/with-meta.jpg', 10, 100, '.jpg')
			`)
			if err != nil {
				t.Fatal(err)
			}
			withMetaID, err := res.LastInsertId()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`
				INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
				VALUES ('file_missing_meta', 'missing-meta', '/library/missing-meta.jpg', 20, 200, '.jpg')
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`
				INSERT INTO media_metadata (location_id, media_kind, mime_type, image_width, updated_at)
				VALUES (?, 'photo', 'image/jpeg', 640, '2026-09-01 12:00:00')
			`, withMetaID); err != nil {
				t.Fatal(err)
			}

			// Reproduce the exact long-lived #670 migration 38 rather than only
			// advancing the numeric ledger. Its trigger remains live while the
			// canonical content-identity migration replaces media_metadata.
			if _, err := db.Exec(`
CREATE TRIGGER invalidate_media_metadata_on_location_content_change
AFTER UPDATE OF content_hash ON locations
WHEN OLD.content_hash IS NOT NEW.content_hash
BEGIN
    DELETE FROM media_metadata WHERE location_id = NEW.id;
END;
`); err != nil {
				t.Fatalf("create historical collision trigger: %v", err)
			}

			// Reproduce the long-lived #670 database state: its historical,
			// unrelated migration 38 left media_metadata location-keyed but
			// advanced the numeric ledger. Some branch databases stopped at 38;
			// later ones applied migration 39 without ever running the replacement
			// content-identity migration 38.
			if err := setMigrationVersion(db, historicalMediaMetadataCollisionVersion, false); err != nil {
				t.Fatal(err)
			}
			if collidedVersion == 39 {
				if err := applyMigration(db, migration39); err != nil {
					t.Fatalf("apply migration 39: %v", err)
				}
			}

			store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}
			if _, err := store.ListPendingMediaMetadataFiles(0, 10); err == nil || !strings.Contains(err.Error(), "mm.content_hash") {
				t.Fatalf("pre-repair sweep error = %v, want missing mm.content_hash", err)
			}

			if err := RunMigrations(db); err != nil {
				t.Fatalf("RunMigrations repair: %v", err)
			}

			version, dirty, err := currentMigrationVersion(db)
			if err != nil {
				t.Fatal(err)
			}
			if version != latestVersion || dirty {
				t.Fatalf("schema_migrations = (%d, %t), want (%d, false)", version, dirty, latestVersion)
			}

			var legacyTriggerCount int
			if err := db.QueryRow(`
				SELECT COUNT(*) FROM sqlite_master
				WHERE type = 'trigger' AND name = 'invalidate_media_metadata_on_location_content_change'
			`).Scan(&legacyTriggerCount); err != nil {
				t.Fatal(err)
			}
			if legacyTriggerCount != 0 {
				t.Fatalf("legacy collision trigger count = %d, want 0", legacyTriggerCount)
			}

			var hash, kind, mime string
			var width int
			if err := db.QueryRow(`SELECT content_hash, media_kind, mime_type, image_width FROM media_metadata`).Scan(&hash, &kind, &mime, &width); err != nil {
				t.Fatal(err)
			}
			if hash != "with-meta" || kind != "photo" || mime != "image/jpeg" || width != 640 {
				t.Fatalf("preserved metadata = (%q, %q, %q, %d)", hash, kind, mime, width)
			}

			pending, err := store.ListPendingMediaMetadataFiles(0, 10)
			if err != nil {
				t.Fatalf("post-repair media metadata sweep: %v", err)
			}
			if len(pending) != 1 || pending[0].Path != "/library/missing-meta.jpg" {
				t.Fatalf("post-repair pending files = %#v, want only missing-meta.jpg", pending)
			}
		})
	}
}
