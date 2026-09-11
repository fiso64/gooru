package database

import (
	"path/filepath"
	"testing"
)

func TestForegroundTransactionReadThenWriteSurvivesConcurrentCommit(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "gooru.db"), false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	store.DB.SetMaxOpenConns(4)

	if _, err := store.Exec(`CREATE TABLE tx_upgrade (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := store.Exec(`INSERT INTO tx_upgrade(id, value) VALUES (1, 'initial')`); err != nil {
		t.Fatalf("seed row: %v", err)
	}

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("begin foreground transaction: %v", err)
	}
	defer tx.Rollback()

	var value string
	if err := tx.QueryRow(`SELECT value FROM tx_upgrade WHERE id = 1`).Scan(&value); err != nil {
		t.Fatalf("establish read snapshot: %v", err)
	}
	if value != "initial" {
		t.Fatalf("initial value = %q, want initial", value)
	}

	// WAL allows this writer to commit while the first transaction retains its
	// read snapshot. A later write in the first transaction must not surface a
	// SQLITE_BUSY/SNAPSHOT error to foreground callers.
	if _, err := store.DB.Exec(`UPDATE tx_upgrade SET value = 'concurrent' WHERE id = 1`); err != nil {
		t.Fatalf("concurrent writer: %v", err)
	}

	if _, err := tx.Exec(`UPDATE tx_upgrade SET value = 'foreground' WHERE id = 1`); err != nil {
		t.Fatalf("read-then-write foreground transaction failed after concurrent commit: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit foreground transaction: %v", err)
	}
}
