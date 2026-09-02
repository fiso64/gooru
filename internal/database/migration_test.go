package database

import (
	"database/sql"
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
