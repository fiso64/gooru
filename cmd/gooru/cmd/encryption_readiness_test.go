package cmd

import (
	"errors"
	"testing"

	"gooru.local/internal/serve"
)

func TestEnsureStorageEncryptionReadyAllowsOrdinaryMode(t *testing.T) {
	cfg := serve.DefaultConfig(t.TempDir() + "/gooru.db")
	if err := ensureStorageEncryptionReady(cfg); err != nil {
		t.Fatalf("ordinary mode should remain available: %v", err)
	}
}

func TestEnsureStorageEncryptionReadyRejectsPartialProtectedMode(t *testing.T) {
	cfg := serve.DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.Encryption.Enabled = true
	if err := ensureStorageEncryptionReady(cfg); !errors.Is(err, errStorageEncryptionIncomplete) {
		t.Fatalf("expected incomplete storage-encryption error, got %v", err)
	}
}
