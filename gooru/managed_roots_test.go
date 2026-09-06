package gooru

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
	"gooru.local/types"
)

func TestRecoverMissingManagedPathRequiresContentIdentity(t *testing.T) {
	t.Run("recovers a moved legacy upload", func(t *testing.T) {
		client, file, oldPath := trackedTestFile(t, []byte("same-content"))
		defer client.Close()

		newRoot := filepath.Join(t.TempDir(), "uploads")
		if err := os.MkdirAll(newRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		newPath := filepath.Join(newRoot, filepath.Base(oldPath))
		if err := os.Rename(oldPath, newPath); err != nil {
			t.Fatal(err)
		}

		repaired, ok, err := client.RecoverMissingManagedPath(file, []string{newRoot})
		if err != nil {
			t.Fatalf("recover moved path: %v", err)
		}
		if !ok {
			t.Fatal("expected moved legacy path to be recovered")
		}
		if repaired.Path != newPath {
			t.Fatalf("repaired path=%q want %q", repaired.Path, newPath)
		}
		if repaired.Size != int64(len("same-content")) {
			t.Fatalf("repaired size=%d", repaired.Size)
		}
	})

	t.Run("rejects a same-name file with different content", func(t *testing.T) {
		client, file, oldPath := trackedTestFile(t, []byte("expected-content"))
		defer client.Close()

		if err := os.Remove(oldPath); err != nil {
			t.Fatal(err)
		}
		newRoot := filepath.Join(t.TempDir(), "uploads")
		if err := os.MkdirAll(newRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		candidate := filepath.Join(newRoot, filepath.Base(oldPath))
		if err := os.WriteFile(candidate, []byte("different-content"), 0o600); err != nil {
			t.Fatal(err)
		}

		repaired, ok, err := client.RecoverMissingManagedPath(file, []string{newRoot})
		if err != nil {
			t.Fatalf("check false candidate: %v", err)
		}
		if ok {
			t.Fatal("different-content candidate must not be accepted")
		}
		if repaired.Path != oldPath {
			t.Fatalf("path changed to %q despite hash mismatch", repaired.Path)
		}
		stored, err := client.GetFileInfoByLocationID(file.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Path != oldPath {
			t.Fatalf("database path changed to %q despite hash mismatch", stored.Path)
		}
	})
}

func TestRecoverMissingManagedPathUsesProtectedLogicalContent(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	oldRoot := filepath.Join(root, "old-uploads")
	newRoot := filepath.Join(root, "new-uploads")
	for _, dir := range []string{oldRoot, newRoot} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	key := bytes.Repeat([]byte{0x73}, 32)
	client, err := NewWithOptions(dbPath, false, OpenOptions{Content: ContentSourceOptions{
		EncryptionKey:  key,
		ProtectedRoots: []string{oldRoot, newRoot},
	}})
	if err != nil {
		t.Fatalf("open protected client: %v", err)
	}
	defer client.Close()

	payload := bytes.Repeat([]byte("relocated-protected-content-"), 512)
	oldPath := filepath.Join(oldRoot, "move-me.bin")
	out, err := os.OpenFile(oldPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(out, bytes.NewReader(payload), int64(len(payload)), key); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	hash, err := client.hasher.HashSource(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("hash plaintext source: %v", err)
	}
	meta, err := client.hasher.FileMetadata(oldPath)
	if err != nil {
		t.Fatalf("protected metadata: %v", err)
	}
	if _, err := client.store.GetOrCreateContent(client.store, hash); err != nil {
		t.Fatalf("create content: %v", err)
	}
	if err := client.store.GetOrCreateLocation(client.store, hash, oldPath, meta.Size, meta.ModTime.Unix(), filepath.Ext(oldPath)); err != nil {
		t.Fatalf("create location: %v", err)
	}
	file, err := client.GetFileInfoByPath(oldPath)
	if err != nil {
		t.Fatalf("load protected location: %v", err)
	}

	newPath := filepath.Join(newRoot, filepath.Base(oldPath))
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	repaired, ok, err := client.RecoverMissingManagedPath(file, []string{newRoot})
	if err != nil {
		t.Fatalf("recover protected moved path: %v", err)
	}
	if !ok {
		t.Fatal("expected protected moved path to be recovered")
	}
	if repaired.Path != newPath || repaired.Hash != hash || repaired.Size != int64(len(payload)) {
		t.Fatalf("protected repaired file = %+v", repaired)
	}
}

func trackedTestFile(t *testing.T, payload []byte) (*Client, types.FileInfo, string) {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}

	oldRoot := filepath.Join(root, "old-uploads")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		client.Close()
		t.Fatal(err)
	}
	oldPath := filepath.Join(oldRoot, "move-me.bin")
	if err := os.WriteFile(oldPath, payload, 0o600); err != nil {
		client.Close()
		t.Fatal(err)
	}
	hash, err := client.hasher.HashFile(oldPath)
	if err != nil {
		client.Close()
		t.Fatalf("hash file: %v", err)
	}
	meta, err := client.hasher.FileMetadata(oldPath)
	if err != nil {
		client.Close()
		t.Fatalf("metadata: %v", err)
	}
	if _, err := client.store.GetOrCreateContent(client.store, hash); err != nil {
		client.Close()
		t.Fatalf("create content: %v", err)
	}
	if err := client.store.GetOrCreateLocation(client.store, hash, oldPath, meta.Size, meta.ModTime.Unix(), filepath.Ext(oldPath)); err != nil {
		client.Close()
		t.Fatalf("create location: %v", err)
	}
	file, err := client.GetFileInfoByPath(oldPath)
	if err != nil {
		client.Close()
		t.Fatalf("load location: %v", err)
	}
	return client, file, oldPath
}
