package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/internal/serve"
)

func TestEnsureStorageEncryptionReadyRemovesPlaintextDerivatives(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media-cache")
	derivative := filepath.Join(root, "ab", strings.Repeat("a", 64)+".jpg")
	if err := os.MkdirAll(filepath.Dir(derivative), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(derivative, []byte("old plaintext thumbnail"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x61}, 32)
	cfg.Media.CacheDir = root
	if err := ensureStorageEncryptionReady(cfg, fakeRegisteredFiles{}); err != nil {
		t.Fatalf("protected storage preflight: %v", err)
	}
	if _, err := os.Stat(derivative); !os.IsNotExist(err) {
		t.Fatalf("protected startup left plaintext derivative behind, stat error = %v", err)
	}
}

func TestEnsureStorageEncryptionReadyOrdinaryModeLeavesCacheUntouched(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media-cache")
	derivative := filepath.Join(root, "ab", strings.Repeat("b", 64)+".png")
	if err := os.MkdirAll(filepath.Dir(derivative), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(derivative, []byte("ordinary thumbnail"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = root
	if err := ensureStorageEncryptionReady(cfg, fakeRegisteredFiles{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(derivative); err != nil {
		t.Fatalf("ordinary startup removed derivative: %v", err)
	}
}
