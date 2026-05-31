package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStoreSecuresSQLiteFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	store, err := NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := store.Exec(`INSERT INTO t DEFAULT VALUES`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("mode for %s = %o, want 600", path, got)
		}
	}
}
