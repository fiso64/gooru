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
	if version != 6 || dirty {
		t.Fatalf("schema_migrations = (%d, %t), want (6, false)", version, dirty)
	}

	if err := RunMigrations(db); err != nil {
		t.Fatalf("second RunMigrations should be a no-op: %v", err)
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
