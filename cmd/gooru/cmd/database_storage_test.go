package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/gooru"
	"gooru.local/internal/database"
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
