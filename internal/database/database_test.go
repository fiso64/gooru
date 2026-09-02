package database

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLocationPublicIDsArePersistedAndResolved(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := CreateEmptyDB(dbPath); err != nil {
		t.Fatalf("CreateEmptyDB: %v", err)
	}
	store, err := NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	if err := RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, "hash-one"); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	if err := store.GetOrCreateLocation(store.DB, "hash-one", "/library/photo.jpg", 123, 456, ".jpg"); err != nil {
		t.Fatalf("GetOrCreateLocation: %v", err)
	}

	file, err := store.GetFileInfoByPath("/library/photo.jpg")
	if err != nil {
		t.Fatalf("GetFileInfoByPath: %v", err)
	}
	if file.PublicID == "" {
		t.Fatal("expected public id")
	}
	if strings.Contains(file.PublicID, "loc:") || strings.Contains(file.PublicID, "/library/photo.jpg") {
		t.Fatalf("public id exposed internal data: %q", file.PublicID)
	}
	resolved, err := store.GetLocationIDByPublicID(file.PublicID)
	if err != nil {
		t.Fatalf("GetLocationIDByPublicID: %v", err)
	}
	if resolved != file.ID {
		t.Fatalf("resolved id = %d, want %d", resolved, file.ID)
	}
	got, err := store.GetLocationPublicID(file.ID)
	if err != nil {
		t.Fatalf("GetLocationPublicID: %v", err)
	}
	if got != file.PublicID {
		t.Fatalf("public id lookup = %q, want %q", got, file.PublicID)
	}
}
