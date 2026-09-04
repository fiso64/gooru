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

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	var version int
	var dirty bool
	if err := db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if version != 8 || dirty {
		t.Fatalf("schema_migrations = (%d, %t), want (8, false)", version, dirty)
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
	if explicit != 4321 {
		t.Fatalf("explicit added_at = %d, want 4321", explicit)
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
	if automatic <= 0 || automatic == 99 {
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
	if addedAt != 2468 {
		t.Fatalf("backfilled added_at = %d, want legacy mod_time 2468", addedAt)
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
