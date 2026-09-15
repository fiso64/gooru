package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func TestConfiguredClientRecoversInterruptedLegacyKeyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(path, types.StrategyPartial, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	master := bytes.Repeat([]byte{0x79}, 32)
	if err := database.MigratePlaintextDatabase(path, master); err != nil {
		t.Fatalf("create legacy-key protected database: %v", err)
	}

	backupPath := path + ".gooru-legacy-key-backup"
	targetPath := path + ".gooru-rekeyed"
	if err := os.Rename(path, backupPath); err != nil {
		t.Fatalf("simulate staged legacy-key database: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("interrupted rekey target"), 0600); err != nil {
		t.Fatalf("simulate interrupted rekey target: %v", err)
	}

	cfg := serve.DefaultConfig(path)
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = master
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("recover interrupted configured database rekey: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close recovered configured client: %v", err)
	}

	keys, err := encryptionkeys.Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := database.NewEncryptedStore(path, false, keys.Database)
	if err != nil {
		t.Fatalf("derived database key does not open recovered database: %v", err)
	}
	if err := migrated.Close(); err != nil {
		t.Fatal(err)
	}
	if legacy, err := database.NewEncryptedStore(path, false, master); err == nil {
		_ = legacy.Close()
		t.Fatal("legacy master key unexpectedly opens recovered database")
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("legacy-key rollback artifact remains after recovery: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("rekey target artifact remains after recovery: %v", err)
	}
}
