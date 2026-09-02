package database

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testEncryptionKey(seed byte) []byte {
	key := make([]byte, encryptedDatabaseKeySize)
	for i := range key {
		key[i] = seed + byte(i)
	}
	return key
}

func TestEncryptedStoreHidesSQLiteHeaderAndData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted.db")
	key := testEncryptionKey(3)
	store, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`CREATE TABLE secrets (value TEXT NOT NULL); INSERT INTO secrets(value) VALUES ('roommate-should-not-see-this-marker')`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(raw, sqliteFileHeader) {
		t.Fatal("encrypted database exposes the plaintext SQLite header")
	}
	if bytes.Contains(raw, []byte("roommate-should-not-see-this-marker")) {
		t.Fatal("encrypted database exposes inserted plaintext")
	}
	if plain, err := IsPlaintextDatabase(path); err != nil || plain {
		t.Fatalf("IsPlaintextDatabase = %t, %v; want false, nil", plain, err)
	}

	reopened, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatalf("reopen encrypted store: %v", err)
	}
	defer reopened.Close()
	var value string
	if err := reopened.DB.QueryRow(`SELECT value FROM secrets`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "roommate-should-not-see-this-marker" {
		t.Fatalf("reopened value = %q", value)
	}
}

func TestEncryptedStoreRejectsWrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "encrypted.db")
	store, err := NewEncryptedStore(path, false, testEncryptionKey(7))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`CREATE TABLE protected (id INTEGER PRIMARY KEY)`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	wrong, err := NewEncryptedStore(path, false, testEncryptionKey(99))
	if err == nil {
		_ = wrong.Close()
		t.Fatal("opening encrypted database with the wrong key unexpectedly succeeded")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("wrong-key open modified encrypted database bytes")
	}
}

func TestMigratePlaintextDatabasePreservesDataAndRemovesPlaintextArtifacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	plain, err := NewStore(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.DB.Exec(`CREATE TABLE migration_secret (value TEXT NOT NULL); INSERT INTO migration_secret(value) VALUES ('plaintext-migration-marker')`); err != nil {
		_ = plain.Close()
		t.Fatal(err)
	}
	if err := plain.Close(); err != nil {
		t.Fatal(err)
	}
	isPlain, err := IsPlaintextDatabase(path)
	if err != nil || !isPlain {
		t.Fatalf("pre-migration plaintext detection = %t, %v", isPlain, err)
	}

	key := testEncryptionKey(11)
	if err := MigratePlaintextDatabase(path, key); err != nil {
		t.Fatalf("MigratePlaintextDatabase: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("plaintext-migration-marker")) || bytes.HasPrefix(raw, sqliteFileHeader) {
		t.Fatal("migrated database still exposes plaintext database content")
	}

	encrypted, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if err := encrypted.DB.QueryRow(`SELECT value FROM migration_secret`).Scan(&value); err != nil {
		_ = encrypted.Close()
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	if value != "plaintext-migration-marker" {
		t.Fatalf("migrated value = %q", value)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "library.db" {
			t.Fatalf("migration left unexpected plaintext/temporary artifact %q", entry.Name())
		}
	}
}

func TestMigratePlaintextDatabaseRefusesEncryptedInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "already-encrypted.db")
	key := testEncryptionKey(13)
	store, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := MigratePlaintextDatabase(path, key); !errors.Is(err, ErrDatabaseNotPlaintext) {
		t.Fatalf("MigratePlaintextDatabase error = %v, want ErrDatabaseNotPlaintext", err)
	}
}
