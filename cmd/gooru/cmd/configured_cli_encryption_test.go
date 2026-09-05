package cmd

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func writeProtectedCLIConfig(t *testing.T, dbPath string, master []byte) string {
	t.Helper()
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "encryption.key")
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(master)+"\n"), 0600); err != nil {
		t.Fatalf("write encryption key: %v", err)
	}
	path := filepath.Join(dir, "gooru.yml")
	data := fmt.Sprintf("database:\n  path: %q\nencryption:\n  enabled: true\n  key_file: %q\n", dbPath, keyPath)
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestNormalCLIPreRunOpensEncryptedDatabaseFromConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatal(err)
	}
	path := writeProtectedCLIConfig(t, dbPath, bytes.Repeat([]byte{0x71}, 32))
	cfg, err := serve.LoadConfig(path, dbPath, serve.Overrides{})
	if err != nil {
		t.Fatalf("load protected config: %v", err)
	}
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("migrate protected database: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	previousConfigPath, previousDatabasePath, previousSvc := configPath, databasePath, svc
	configPath, databasePath, svc = path, "", nil
	t.Cleanup(func() {
		if svc != nil {
			_ = svc.Close()
		}
		configPath, databasePath, svc = previousConfigPath, previousDatabasePath, previousSvc
	})

	if err := rootCmd.PersistentPreRunE(listCmd, nil); err != nil {
		t.Fatalf("normal CLI pre-run did not open encrypted database: %v", err)
	}
	if svc == nil {
		t.Fatal("normal CLI pre-run did not initialize the configured client")
	}
	if _, err := svc.GetAllFilesInfo(); err != nil {
		t.Fatalf("ordinary CLI client operation failed on encrypted database: %v", err)
	}
	rootCmd.PersistentPostRun(listCmd, nil)
}

func TestPrepareAdminDatabaseUsesEncryptedStoragePolicy(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatal(err)
	}
	path := writeProtectedCLIConfig(t, dbPath, bytes.Repeat([]byte{0x72}, 32))
	cfg, err := serve.LoadConfig(path, dbPath, serve.Overrides{})
	if err != nil {
		t.Fatalf("load protected config: %v", err)
	}
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("migrate protected database: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := prepareAdminDatabase(cfg, false)
	if err != nil {
		t.Fatalf("prepare admin database with protected config: %v", err)
	}
	defer store.Close()
	if err := store.DB.Ping(); err != nil {
		t.Fatalf("ping protected admin database: %v", err)
	}
}

func TestConfiguredDatabasePathUsesYAMLConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "configured.db")
	path := writeProtectedCLIConfig(t, dbPath, bytes.Repeat([]byte{0x73}, 32))
	previousConfigPath, previousDatabasePath := configPath, databasePath
	configPath, databasePath = path, ""
	t.Cleanup(func() {
		configPath, databasePath = previousConfigPath, previousDatabasePath
	})

	got, err := configuredDatabasePath()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(got) != dbPath {
		t.Fatalf("configuredDatabasePath() = %q, want %q", got, dbPath)
	}
}
