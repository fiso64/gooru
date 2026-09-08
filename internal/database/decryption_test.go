package database

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateEncryptedDatabaseToPlaintextPreservesDataAndRemovesEncryptedArtifacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	key := testEncryptionKey(21)
	encrypted, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encrypted.DB.Exec(`CREATE TABLE migration_secret (value TEXT NOT NULL); INSERT INTO migration_secret(value) VALUES ('restored-plaintext-marker')`); err != nil {
		_ = encrypted.Close()
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte("restored-plaintext-marker")) || bytes.HasPrefix(before, sqliteFileHeader) {
		t.Fatal("pre-migration database unexpectedly exposes plaintext")
	}

	if err := MigrateEncryptedDatabaseToPlaintext(path, key); err != nil {
		t.Fatalf("MigrateEncryptedDatabaseToPlaintext: %v", err)
	}
	plain, err := IsPlaintextDatabase(path)
	if err != nil || !plain {
		t.Fatalf("post-migration plaintext detection = %t, %v", plain, err)
	}
	store, err := NewStore(path, false)
	if err != nil {
		t.Fatal(err)
	}
	var value string
	if err := store.DB.QueryRow(`SELECT value FROM migration_secret`).Scan(&value); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if value != "restored-plaintext-marker" {
		t.Fatalf("restored value = %q", value)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "library.db" {
			t.Fatalf("migration left unexpected backup/temporary artifact %q", entry.Name())
		}
	}
}

func TestMigrateEncryptedDatabaseToPlaintextWrongKeyLeavesCiphertextUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	key := testEncryptionKey(31)
	encrypted, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encrypted.DB.Exec(`CREATE TABLE protected (value TEXT NOT NULL); INSERT INTO protected(value) VALUES ('keep-encrypted')`); err != nil {
		_ = encrypted.Close()
		t.Fatal(err)
	}
	if err := encrypted.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := MigrateEncryptedDatabaseToPlaintext(path, testEncryptionKey(32)); err == nil {
		t.Fatal("wrong-key plaintext migration unexpectedly succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("wrong-key migration mutated encrypted database")
	}
	if plain, err := IsPlaintextDatabase(path); err != nil || plain {
		t.Fatalf("database plaintext after failed migration = %t, %v", plain, err)
	}
}

func TestMigrateEncryptedDatabaseToPlaintextIsIdempotentForPlaintext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	plain, err := NewStore(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plain.DB.Exec(`CREATE TABLE ordinary (value TEXT NOT NULL); INSERT INTO ordinary(value) VALUES ('already-plain')`); err != nil {
		_ = plain.Close()
		t.Fatal(err)
	}
	if err := plain.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateEncryptedDatabaseToPlaintext(path, testEncryptionKey(41)); err != nil {
		t.Fatalf("idempotent plaintext migration: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("idempotent plaintext migration changed database")
	}
}
