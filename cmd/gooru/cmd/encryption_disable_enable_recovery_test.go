package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func TestConfiguredDisableRecoversInterruptedPlaintextEncryptionMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(path, types.StrategyFull, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	backupPath := path + ".gooru-enable-plaintext-backup"
	targetPath := path + ".gooru-enable-encrypted"
	if err := os.Rename(path, backupPath); err != nil {
		t.Fatalf("simulate staged plaintext database: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("interrupted encrypted target"), 0600); err != nil {
		t.Fatalf("simulate interrupted encrypted target: %v", err)
	}

	cfg := serve.DefaultConfig(path)
	cfg.Encryption.Enabled = false
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("open after interrupted encryption enable was disabled: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close recovered plaintext client: %v", err)
	}

	plain, err := database.IsPlaintextDatabase(path)
	if err != nil {
		t.Fatalf("inspect recovered database: %v", err)
	}
	if !plain {
		t.Fatal("recovered database is not plaintext after disabled encryption startup")
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("plaintext rollback artifact remains after recovery: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("encrypted target artifact remains after recovery: %v", err)
	}

	reopened, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("ordinary restart after recovered plaintext database: %v", err)
	}
	defer reopened.Close()
}
