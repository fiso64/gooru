package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func TestConfiguredDatabaseOpenersShareEncryptedStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(path, types.StrategyFull, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	cfg := serve.DefaultConfig(path)
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x31}, 32)

	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("open configured encrypted client: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close configured encrypted client: %v", err)
	}

	plain, err := database.IsPlaintextDatabase(path)
	if err != nil {
		t.Fatalf("inspect configured database: %v", err)
	}
	if plain {
		t.Fatal("configured encrypted client left the database plaintext")
	}
	keys, err := encryptionkeys.Derive(cfg.Encryption.Key)
	if err != nil {
		t.Fatal(err)
	}
	if legacy, err := database.NewEncryptedStore(path, false, cfg.Encryption.Key); err == nil {
		_ = legacy.Close()
		t.Fatal("master key unexpectedly opens database after domain separation")
	}
	derived, err := database.NewEncryptedStore(path, false, keys.Database)
	if err != nil {
		t.Fatalf("database subkey does not open configured database: %v", err)
	}
	if err := derived.Close(); err != nil {
		t.Fatal(err)
	}

	authStore, err := openConfiguredAuthStore(cfg, false)
	if err != nil {
		t.Fatalf("open auth store on encrypted database: %v", err)
	}
	if err := authStore.DB.Ping(); err != nil {
		t.Fatalf("ping auth store: %v", err)
	}
	if err := authStore.Close(); err != nil {
		t.Fatalf("close auth store: %v", err)
	}
}

func TestConfiguredClientMigratesLegacyMasterKeyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(path, types.StrategyPartial, false); err != nil {
		t.Fatal(err)
	}
	master := bytes.Repeat([]byte{0x47}, 32)
	if err := database.MigratePlaintextDatabase(path, master); err != nil {
		t.Fatalf("create legacy master-key database: %v", err)
	}
	legacy, err := database.NewEncryptedStore(path, false, master)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.SetFileCount(17); err != nil {
		_ = legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(path)
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = master
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("open legacy protected database: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	keys, err := encryptionkeys.Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	migrated, err := database.NewEncryptedStore(path, false, keys.Database)
	if err != nil {
		t.Fatalf("derived database key does not open migrated database: %v", err)
	}
	defer migrated.Close()
	if legacy, err := database.NewEncryptedStore(path, false, master); err == nil {
		_ = legacy.Close()
		t.Fatal("legacy master key unexpectedly opens migrated database")
	}
}

func TestConfiguredAuthStoreUsesEncryptionKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(path, types.StrategyPartial, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	cfg := serve.DefaultConfig(path)
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x52}, 32)
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("migrate configured database: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	wrong := cfg
	wrong.Encryption.Key = bytes.Repeat([]byte{0x53}, 32)
	if store, err := openConfiguredAuthStore(wrong, false); err == nil {
		_ = store.Close()
		t.Fatal("auth store unexpectedly opened encrypted database with wrong key")
	} else if !strings.Contains(err.Error(), "recovery-critical encryption key") || !strings.Contains(err.Error(), "re-key is not supported") {
		t.Fatalf("wrong-key diagnostic is not actionable: %v", err)
	}
}

func TestConfiguredOpenRejectsDisablingEncryptionForEncryptedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gooru.db")
	if err := gooru.Init(path, types.StrategyFull, false); err != nil {
		t.Fatalf("initialize plaintext database: %v", err)
	}

	cfg := serve.DefaultConfig(path)
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x61}, 32)
	client, err := openConfiguredClient(cfg, false)
	if err != nil {
		t.Fatalf("migrate configured database: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}

	disabled := cfg
	disabled.Encryption.Enabled = false
	disabled.Encryption.Key = nil
	if _, err := openConfiguredClient(disabled, false); err == nil {
		t.Fatal("disabled encryption unexpectedly opened encrypted database")
	} else if !strings.Contains(err.Error(), "disabling protected storage in place is not supported") || !strings.Contains(err.Error(), "original recovery key") {
		t.Fatalf("disable diagnostic is not actionable: %v", err)
	}
}
