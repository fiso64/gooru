package gooru

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/database"
	"gooru.local/types"
)

func TestNewWithDatabaseOptionsMigratesPlaintextExplicitly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(path, types.StrategyFull, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}
	plain, err := database.IsPlaintextDatabase(path)
	if err != nil || !plain {
		t.Fatalf("expected plaintext database before migration, plaintext=%v err=%v", plain, err)
	}

	key := bytes.Repeat([]byte{0x42}, 32)
	client, err := NewWithDatabaseOptions(path, false, DatabaseOpenOptions{
		EncryptionKey:    key,
		MigratePlaintext: true,
	})
	if err != nil {
		t.Fatalf("open encrypted client with migration: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close encrypted client: %v", err)
	}

	plain, err = database.IsPlaintextDatabase(path)
	if err != nil {
		t.Fatalf("inspect migrated database: %v", err)
	}
	if plain {
		t.Fatal("database remained plaintext after explicit encrypted migration")
	}

	reopened, err := NewWithDatabaseOptions(path, false, DatabaseOpenOptions{EncryptionKey: key})
	if err != nil {
		t.Fatalf("reopen encrypted client: %v", err)
	}
	defer reopened.Close()
}

func TestNewWithDatabaseOptionsDoesNotImplicitlyMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(path, types.StrategyPartial, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	key := bytes.Repeat([]byte{0x24}, 32)
	if _, err := NewWithDatabaseOptions(path, false, DatabaseOpenOptions{EncryptionKey: key}); err == nil {
		t.Fatal("opening plaintext as encrypted without migration should fail")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed encrypted open modified plaintext database")
	}
}
