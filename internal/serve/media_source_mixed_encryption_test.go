package serve

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
)

func TestProtectedMediaSourceLeavesExternalPlaintextReadable(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(dir, "external.bin")
	plaintext := []byte("external media stays owner-managed")
	if err := os.WriteFile(externalPath, plaintext, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x61}, 32)
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}

	source, err := NewMediaService(cfg).openMediaSource(externalPath)
	if err != nil {
		t.Fatalf("open external plaintext in protected mode: %v", err)
	}
	got, err := io.ReadAll(source)
	if closeErr := source.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("external plaintext media changed while reading")
	}
}

func TestProtectedMediaSourceRejectsPlaintextManagedUpload(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(uploadRoot, "managed.bin")
	if err := os.WriteFile(path, []byte("must be migrated before serving"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x62}, 32)
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}

	if _, err := NewMediaService(cfg).openMediaSource(path); !errors.Is(err, encryptedfile.ErrInvalidFormat) {
		t.Fatalf("plaintext managed upload error = %v, want invalid encrypted format", err)
	}
}

func TestProtectedMediaSourceAuthenticatesEncryptedFileOutsideCurrentTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moved.bin")
	plaintext := []byte("encrypted media remains readable after target configuration changes")
	key := bytes.Repeat([]byte{0x63}, 32)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = key

	source, err := NewMediaService(cfg).openMediaSource(path)
	if err != nil {
		t.Fatalf("open encrypted external media: %v", err)
	}
	got, err := io.ReadAll(source)
	if closeErr := source.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("encrypted external media decrypted incorrectly")
	}
}
