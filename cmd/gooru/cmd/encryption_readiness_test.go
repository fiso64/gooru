package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

type fakeRegisteredFiles struct {
	files    []types.FileInfo
	err      error
	mappings map[int64]string
}

func (f fakeRegisteredFiles) GetAllFilesInfo() ([]types.FileInfo, error) {
	return f.files, f.err
}

func (f fakeRegisteredFiles) ManagedStoragePath(locationID int64) (string, bool, error) {
	path, ok := f.mappings[locationID]
	return path, ok, nil
}

func (f fakeRegisteredFiles) SetManagedStoragePath(locationID int64, physicalPath string) error {
	if f.mappings == nil {
		return errors.New("fake managed storage map is nil")
	}
	f.mappings[locationID] = physicalPath
	return nil
}

func (f fakeRegisteredFiles) ClearManagedStoragePath(locationID int64) error {
	delete(f.mappings, locationID)
	return nil
}

func TestEnsureStorageEncryptionReadyAllowsOrdinaryMode(t *testing.T) {
	cfg := serve.DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if err := ensureStorageEncryptionReady(cfg, fakeRegisteredFiles{}); err != nil {
		t.Fatalf("ordinary mode should remain available: %v", err)
	}
}

func TestEnsureStorageEncryptionReadyMigratesOnlyManagedUploads(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	managedPath := filepath.Join(uploadRoot, "managed.bin")
	externalPath := filepath.Join(dir, "external.bin")
	managedPlaintext := []byte("managed secret")
	externalPlaintext := []byte("external library media")
	if err := os.WriteFile(managedPath, managedPlaintext, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalPath, externalPlaintext, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x41}, 32)
	keys, err := encryptionkeys.Derive(cfg.Encryption.Key)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}
	files := fakeRegisteredFiles{files: []types.FileInfo{{ID: 1, Path: managedPath, Hash: "managed-hash"}, {ID: 2, Path: externalPath, Hash: "external-hash"}}, mappings: map[int64]string{}}

	if err := ensureStorageEncryptionReady(cfg, files); err != nil {
		t.Fatalf("protected storage preflight: %v", err)
	}
	physicalPath := files.mappings[1]
	if physicalPath == "" || filepath.Base(physicalPath) == filepath.Base(managedPath) {
		t.Fatalf("managed upload did not receive opaque physical name: %q", physicalPath)
	}
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("logical managed filename remains on disk: %v", err)
	}
	encrypted, err := encryptedfile.IsEncryptedFile(physicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !encrypted {
		t.Fatal("registered managed upload remained plaintext")
	}
	opened, err := encryptedfile.Open(physicalPath, keys.Media)
	if err != nil {
		t.Fatalf("open migrated managed upload with media subkey: %v", err)
	}
	got, err := io.ReadAll(opened)
	if closeErr := opened.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, managedPlaintext) {
		t.Fatal("migrated managed upload changed plaintext")
	}
	external, err := os.ReadFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(external, externalPlaintext) {
		t.Fatal("protected startup rewrote external library media")
	}

	if err := ensureStorageEncryptionReady(cfg, files); err != nil {
		t.Fatalf("repeated protected storage preflight should be idempotent: %v", err)
	}
}

func TestEnsureStorageEncryptionReadyMigratesLegacyMasterKeyMedia(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(uploadRoot, "legacy.bin")
	plaintext := []byte("legacy master-key media")
	master := bytes.Repeat([]byte{0x4a}, 32)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), master); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = master
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}
	files := fakeRegisteredFiles{files: []types.FileInfo{{ID: 1, Path: path, Hash: "legacy-hash"}}, mappings: map[int64]string{}}
	if err := ensureStorageEncryptionReady(cfg, files); err != nil {
		t.Fatalf("migrate legacy managed media: %v", err)
	}
	physicalPath := files.mappings[1]
	keys, err := encryptionkeys.Derive(master)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := encryptedfile.Open(physicalPath, keys.Media)
	if err != nil {
		t.Fatalf("open migrated media with derived key: %v", err)
	}
	got, err := io.ReadAll(opened)
	if closeErr := opened.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("legacy media migration changed plaintext")
	}
	if legacy, err := encryptedfile.Open(physicalPath, master); err == nil {
		_ = legacy.Close()
		t.Fatal("legacy master key unexpectedly opens migrated media")
	}
}

func TestEnsureStorageEncryptionReadyDoesNotFollowNestedSymlinkOutsideUploadRoot(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	externalRoot := filepath.Join(dir, "external")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(externalRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(uploadRoot, "linked")
	if err := os.Symlink(externalRoot, linkedDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	externalPath := filepath.Join(externalRoot, "outside.bin")
	registeredPath := filepath.Join(linkedDir, "outside.bin")
	plaintext := []byte("must remain outside gooru managed storage")
	if err := os.WriteFile(externalPath, plaintext, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x42}, 32)
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}
	if err := ensureStorageEncryptionReady(cfg, fakeRegisteredFiles{files: []types.FileInfo{{ID: 1, Path: registeredPath, Hash: "outside-hash"}}, mappings: map[int64]string{}}); err != nil {
		t.Fatalf("protected storage preflight: %v", err)
	}

	after, err := os.ReadFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, plaintext) {
		t.Fatal("protected startup rewrote a file reached through a nested symlink outside the upload root")
	}
	encrypted, err := encryptedfile.IsEncryptedFile(externalPath)
	if err != nil {
		t.Fatal(err)
	}
	if encrypted {
		t.Fatal("external file reached through upload-root symlink was encrypted")
	}
}

func TestEnsureStorageEncryptionReadyFailsClosedForWrongManagedUploadKey(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(uploadRoot, "managed.bin")
	plaintext := []byte("already encrypted managed upload")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	goodKey := bytes.Repeat([]byte{0x51}, 32)
	if err := encryptedfile.Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), goodKey); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x52}, 32)
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Name: "Managed", Path: uploadRoot}}
	err = ensureStorageEncryptionReady(cfg, fakeRegisteredFiles{files: []types.FileInfo{{ID: 1, Path: path, Hash: "wrong-key-hash"}}, mappings: map[int64]string{}})
	if err == nil {
		t.Fatal("protected storage preflight accepted a managed upload encrypted with another key")
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("wrong-key preflight mutated managed upload")
	}
}
