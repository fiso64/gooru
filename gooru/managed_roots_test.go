package gooru

import (
	"os"
	"path/filepath"
	"testing"

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
