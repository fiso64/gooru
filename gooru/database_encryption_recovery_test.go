package gooru

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/database"
	"gooru.local/types"
)

func TestNewWithDatabaseOptionsRecoversInterruptedPlaintextEncryptionMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(path, types.StrategyFull, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	backupPath := path + ".gooru-enable-plaintext-backup"
	targetPath := path + ".gooru-enable-encrypted"
	if err := os.Rename(path, backupPath); err != nil {
		t.Fatalf("simulate staged plaintext source: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("interrupted target"), 0600); err != nil {
		t.Fatalf("simulate interrupted encrypted target: %v", err)
	}

	key := bytes.Repeat([]byte{0x6a}, 32)
	client, err := NewWithDatabaseOptions(path, false, DatabaseOpenOptions{
		EncryptionKey:    key,
		MigratePlaintext: true,
	})
	if err != nil {
		t.Fatalf("recover and reopen protected database: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close recovered protected database: %v", err)
	}

	plain, err := database.IsPlaintextDatabase(path)
	if err != nil {
		t.Fatalf("inspect recovered database: %v", err)
	}
	if plain {
		t.Fatal("recovered database was not migrated to encrypted storage")
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("plaintext rollback artifact remains after recovery: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("encrypted target artifact remains after recovery: %v", err)
	}

	reopened, err := NewWithDatabaseOptions(path, false, DatabaseOpenOptions{EncryptionKey: key})
	if err != nil {
		t.Fatalf("reopen recovered encrypted database: %v", err)
	}
	defer reopened.Close()
}
