package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func TestConfiguredDisableRecoversInterruptedLegacyKeyMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.db")
	if err := gooru.Init(path, types.StrategyPartial, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	master := bytes.Repeat([]byte{0x5d}, 32)
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
	cfg.Encryption.Enabled = false
	cfg.Encryption.KeyFile = writeRecoveryKeyFile(t, dir, master)
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("disable protected storage after interrupted rekey: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close recovered plaintext client: %v", err)
	}

	plain, err := database.IsPlaintextDatabase(path)
	if err != nil {
		t.Fatalf("inspect recovered database: %v", err)
	}
	if !plain {
		t.Fatal("database remained encrypted after recovered disable transition")
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("legacy-key rollback artifact remains after recovered disable: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("rekey target artifact remains after recovered disable: %v", err)
	}

	cfg.Encryption.KeyFile = ""
	reopened, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("ordinary restart after recovered disable: %v", err)
	}
	defer reopened.Close()
}
