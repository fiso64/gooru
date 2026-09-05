package database

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestMigrateEncryptedDatabaseKeyPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	oldKey := bytes.Repeat([]byte{0x21}, encryptedDatabaseKeySize)
	newKey := bytes.Repeat([]byte{0x22}, encryptedDatabaseKeySize)

	store, err := NewEncryptedStore(path, false, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`CREATE TABLE migration_probe (value TEXT NOT NULL); INSERT INTO migration_probe(value) VALUES ('kept')`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := MigrateEncryptedDatabaseKey(path, oldKey, newKey); err != nil {
		t.Fatalf("migrate encrypted database key: %v", err)
	}

	migrated, err := NewEncryptedStore(path, false, newKey)
	if err != nil {
		t.Fatalf("open with new key: %v", err)
	}
	defer migrated.Close()
	var got string
	if err := migrated.DB.QueryRow(`SELECT value FROM migration_probe`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "kept" {
		t.Fatalf("value = %q, want kept", got)
	}

	if legacy, err := NewEncryptedStore(path, false, oldKey); err == nil {
		_ = legacy.Close()
		t.Fatal("legacy key unexpectedly opened migrated database")
	}
}
