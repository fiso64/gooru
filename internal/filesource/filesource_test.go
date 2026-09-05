package filesource

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
)

func TestProtectedResolverReturnsLogicalPlaintext(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "media.bin")
	plaintext := []byte("logical plaintext media")
	key := bytes.Repeat([]byte{0x42}, 32)
	writeEncryptedTestFile(t, path, plaintext, key)

	resolver, err := NewProtected(key, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if source.Size() != int64(len(plaintext)) {
		t.Fatalf("logical size = %d, want %d", source.Size(), len(plaintext))
	}
	got, err := io.ReadAll(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("logical contents = %q, want %q", got, plaintext)
	}
}

func TestProtectedResolverKeepsExternalFilesPlaintext(t *testing.T) {
	root := t.TempDir()
	externalRoot := t.TempDir()
	path := filepath.Join(externalRoot, "external.bin")
	plaintext := []byte("external plaintext")
	if err := os.WriteFile(path, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProtected(bytes.Repeat([]byte{0x23}, 32), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	got, err := io.ReadAll(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("contents = %q, want %q", got, plaintext)
	}
}

func TestProtectedResolverAuthenticatesEncryptedFileOutsideCurrentRoots(t *testing.T) {
	root := t.TempDir()
	externalRoot := t.TempDir()
	path := filepath.Join(externalRoot, "previously-managed.bin")
	plaintext := []byte("encrypted tracked content outside current root")
	key := bytes.Repeat([]byte{0x66}, 32)
	writeEncryptedTestFile(t, path, plaintext, key)

	resolver, err := NewProtected(key, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	got, err := io.ReadAll(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("contents = %q, want %q", got, plaintext)
	}
}

func TestProtectedResolverRejectsPlaintextManagedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "plaintext.bin")
	if err := os.WriteFile(path, []byte("must not leak"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProtected(bytes.Repeat([]byte{0x31}, 32), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolver.Open(path)
	if !errors.Is(err, encryptedfile.ErrInvalidFormat) {
		t.Fatalf("Open() error = %v, want invalid encrypted format", err)
	}
}

func TestProtectedPathDoesNotMatchSiblingPrefix(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "managed")
	sibling := filepath.Join(base, "managed-other", "file.bin")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	resolver, err := NewProtected(bytes.Repeat([]byte{0x51}, 32), []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.IsProtectedPath(sibling) {
		t.Fatalf("sibling-prefix path %q was treated as protected", sibling)
	}
}

func writeEncryptedTestFile(t *testing.T, path string, plaintext, key []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := encryptedfile.Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
