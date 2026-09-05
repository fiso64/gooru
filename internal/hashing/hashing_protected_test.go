package hashing

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/filesource"
	"gooru.local/types"
)

func TestHashFileUsesLogicalPlaintextForProtectedPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "managed.bin")
	plaintext := bytes.Repeat([]byte("gooru-protected-content-"), 8192)
	key := bytes.Repeat([]byte{0x72}, 32)

	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(out, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	resolver, err := filesource.NewProtected(key, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := NewHasher(types.StrategyPartial)
	if err != nil {
		t.Fatal(err)
	}
	hasher.SetSourceResolver(resolver)

	got, err := hasher.HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := hasher.HashSource(bytes.NewReader(plaintext), int64(len(plaintext)))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("protected HashFile() = %q, plaintext hash = %q", got, want)
	}

	metadata, err := hasher.FileMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Size != int64(len(plaintext)) {
		t.Fatalf("logical size = %d, want %d", metadata.Size, len(plaintext))
	}
	physical, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Size == physical.Size() {
		t.Fatalf("test did not distinguish logical size %d from encrypted size %d", metadata.Size, physical.Size())
	}
}
