package database

import (
	"database/sql"
	"strings"
	"testing"
)

func TestRunMigrationsUsesSuppliedConnection(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'contents'`).Scan(&count); err != nil {
		t.Fatalf("query migrated schema: %v", err)
	}
	if count != 1 {
		t.Fatalf("contents table count = %d, want 1 on supplied connection", count)
	}

	if err := db.Ping(); err != nil {
		t.Fatalf("RunMigrations closed caller-owned database: %v", err)
	}
}

func TestRunMigrationsPreservesGolangMigrateVersionLayout(t *testing.T) {
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

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	var version int
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != latestVersion || dirty {
		t.Fatalf("schema_migrations = (%d, %t), want (%d, false)", version, dirty, latestVersion)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations should be a no-op: %v", err)
	}
}

func TestLocationAddedAtMigrationBackfillsAndDefaultsNewRows(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('existing')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension, added_at)
		VALUES ('file_existing', 'existing', '/existing.jpg', 1, 1234, '.jpg', 4321)
	`); err != nil {
		t.Fatal(err)
	}

	var explicit int64
	if err := db.QueryRow(`SELECT added_at FROM locations WHERE public_id = 'file_existing'`).Scan(&explicit); err != nil {
		t.Fatal(err)
	}
	if explicit != 4321*1000 {
		t.Fatalf("explicit added_at = %d, want %d", explicit, int64(4321*1000))
	}

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('new')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES ('file_new', 'new', '/new.jpg', 1, 99, '.jpg')
	`); err != nil {
		t.Fatal(err)
	}

	var automatic int64
	if err := db.QueryRow(`SELECT added_at FROM locations WHERE public_id = 'file_new'`).Scan(&automatic); err != nil {
		t.Fatal(err)
	}
	if automatic < 100000000000 || automatic == 99*1000 {
		t.Fatalf("automatic added_at = %d, want current Gooru insertion time independent of mod_time", automatic)
	}
}

func TestLocationAddedAtMigrationBackfillsExistingRows(t *testing.T) {
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
		if migration.version >= 8 {
			break
		}
		if err := applyMigration(db, migration); err != nil {
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('legacy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES ('file_legacy', 'legacy', '/legacy.jpg', 1, 2468, '.jpg')
	`); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations through v8: %v", err)
	}
	var addedAt int64
	if err := db.QueryRow(`SELECT added_at FROM locations WHERE public_id = 'file_legacy'`).Scan(&addedAt); err != nil {
		t.Fatal(err)
	}
	if addedAt != 2468*1000 {
		t.Fatalf("backfilled added_at = %d, want legacy mod_time %d", addedAt, int64(2468*1000))
	}
}

func TestFilenameTrigramIndexBackfillsAndTracksLocations(t *testing.T) {
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
		if migration.version >= 10 {
			break
		}
		if err := applyMigration(db, migration); err != nil {
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
	}

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('legacy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES ('file_legacy', 'legacy', '/nested/Legacy-Photo.JPG', 1, 1, '.JPG')
	`); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations through v10: %v", err)
	}

	assertFilename := func(id int64, want string) {
		t.Helper()
		var got string
		if err := db.QueryRow(`SELECT filename FROM location_filenames WHERE rowid = ?`, id).Scan(&got); err != nil {
			t.Fatalf("read filename row %d: %v", id, err)
		}
		if got != want {
			t.Fatalf("filename row %d = %q, want %q", id, got, want)
		}
	}

	var legacyID int64
	if err := db.QueryRow(`SELECT id FROM locations WHERE public_id = 'file_legacy'`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	assertFilename(legacyID, "Legacy-Photo.JPG")

	if _, err := db.Exec(`INSERT INTO contents (hash) VALUES ('new')`); err != nil {
		t.Fatal(err)
	}
	res, err := db.Exec(`
		INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, extension)
		VALUES ('file_new', 'new', 'C:\\pictures\\Fresh_File.png', 1, 1, '.png')
	`)
	if err != nil {
		t.Fatal(err)
	}
	newID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	assertFilename(newID, "Fresh_File.png")

	if _, err := db.Exec(`UPDATE locations SET path = '/renamed/Final.Name.webp' WHERE id = ?`, newID); err != nil {
		t.Fatal(err)
	}
	assertFilename(newID, "Final.Name.webp")

	if _, err := db.Exec(`DELETE FROM locations WHERE id = ?`, newID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM location_filenames WHERE rowid = ?`, newID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("deleted location left %d filename index rows", count)
	}
}

func TestRunMigrationsRefusesDirtyDatabase(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := ensureMigrationTable(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (3, true)`); err != nil {
		t.Fatal(err)
	}

	err = RunMigrations(db)
	if err == nil || !strings.Contains(err.Error(), "version 3 is dirty") {
		t.Fatalf("RunMigrations error = %v, want dirty migration refusal", err)
	}
}
