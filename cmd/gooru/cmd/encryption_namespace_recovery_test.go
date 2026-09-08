package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

func TestEnsureStorageEncryptionReadyRemovesFreshProtectedUploadOrphan(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x61}, 32)
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Path: uploadRoot}}

	orphan, err := serve.OpaqueManagedStoragePathForTargets(cfg.Uploads.Targets, filepath.Join(uploadRoot, "secret.jpg"), "unused", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("stranded protected upload"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := fakeRegisteredFiles{mappings: map[int64]string{}}
	if err := ensureStorageEncryptionReady(cfg, files); err != nil {
		t.Fatalf("protected storage startup reconciliation: %v", err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("fresh protected upload orphan still exists: %v", err)
	}
}

func TestEnsureStorageEncryptionReadyMigratesLegacyOpaqueSiblingIntoNamespace(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	logicalDir := filepath.Join(uploadRoot, "nested")
	if err := os.MkdirAll(logicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logicalPath := filepath.Join(logicalDir, "secret.jpg")
	opaqueName := strings.Repeat("ab", 24)
	legacyPath := filepath.Join(logicalDir, opaqueName)
	plaintext := []byte("legacy protected sibling")
	if err := os.WriteFile(legacyPath, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x62}, 32)
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Path: uploadRoot}}
	files := fakeRegisteredFiles{
		files:    []types.FileInfo{{ID: 1, Path: logicalPath, Hash: "legacy-hash"}},
		mappings: map[int64]string{1: legacyPath},
	}

	if err := ensureStorageEncryptionReady(cfg, files); err != nil {
		t.Fatalf("migrate legacy protected sibling: %v", err)
	}
	physicalPath := files.mappings[1]
	wantPath, err := serve.ProtectedManagedStoragePathForName(cfg.Uploads.Targets, logicalPath, opaqueName)
	if err != nil {
		t.Fatal(err)
	}
	if physicalPath != wantPath {
		t.Fatalf("managed mapping = %q, want %q", physicalPath, wantPath)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy sibling still exists: %v", err)
	}
	keys, err := encryptionkeys.Derive(cfg.Encryption.Key)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := encryptedfile.Open(physicalPath, keys.Media)
	if err != nil {
		t.Fatalf("open namespaced migrated media: %v", err)
	}
	defer opened.Close()
	buf := make([]byte, len(plaintext))
	if _, err := opened.ReadAt(buf, 0); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, plaintext) {
		t.Fatal("namespaced migration changed plaintext")
	}
}

func TestEnsureStorageEncryptionReadyResumesCheckpointedLegacyNamespaceMove(t *testing.T) {
	dir := t.TempDir()
	uploadRoot := filepath.Join(dir, "uploads")
	logicalDir := filepath.Join(uploadRoot, "nested")
	if err := os.MkdirAll(logicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logicalPath := filepath.Join(logicalDir, "secret.jpg")
	opaqueName := strings.Repeat("cd", 24)
	legacyPath := filepath.Join(logicalDir, opaqueName)
	plaintext := []byte("checkpointed legacy namespace move")
	if err := os.WriteFile(legacyPath, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := serve.DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x63}, 32)
	cfg.Uploads.Targets = []serve.UploadTarget{{ID: "managed", Path: uploadRoot}}
	physicalPath, err := serve.ProtectedManagedStoragePathForName(cfg.Uploads.Targets, logicalPath, opaqueName)
	if err != nil {
		t.Fatal(err)
	}
	files := fakeRegisteredFiles{
		files:    []types.FileInfo{{ID: 1, Path: logicalPath, Hash: "legacy-hash"}},
		mappings: map[int64]string{1: physicalPath},
	}

	if err := ensureStorageEncryptionReady(cfg, files); err != nil {
		t.Fatalf("resume checkpointed namespace move: %v", err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy sibling still exists after recovery: %v", err)
	}
	if _, err := os.Stat(physicalPath); err != nil {
		t.Fatalf("namespaced protected file missing after recovery: %v", err)
	}
}
