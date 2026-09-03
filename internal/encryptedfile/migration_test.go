package encryptedfile

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEncryptFileInPlaceMigratesPlaintextWithoutChangingLogicalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.bin")
	plaintext := bytes.Repeat([]byte("private-media-"), 100000)
	if err := os.WriteFile(path, plaintext, 0o644); err != nil {
		t.Fatal(err)
	}
	originalTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(path, originalTime, originalTime); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x44}, 32)

	if err := EncryptFileInPlace(path, key); err != nil {
		t.Fatalf("migrate plaintext file: %v", err)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(stored, plaintext) || bytes.Contains(stored, []byte("private-media-")) {
		t.Fatal("encrypted replacement exposes plaintext content")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("encrypted replacement mode = %o, want 600", info.Mode().Perm())
	}
	if !info.ModTime().Equal(originalTime) {
		t.Fatalf("encrypted replacement modtime = %v, want %v", info.ModTime(), originalTime)
	}
	opened, err := Open(path, key)
	if err != nil {
		t.Fatalf("open migrated file: %v", err)
	}
	got, err := io.ReadAll(opened)
	if closeErr := opened.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("migrated plaintext differs from source")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".secret.bin.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("migration left sibling artifacts: %v", matches)
	}
}

func TestEncryptFileInPlaceRejectsAlreadyEncryptedFileWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.bin")
	key := bytes.Repeat([]byte{0x23}, 32)
	plaintext := []byte("already protected")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := EncryptFileInPlace(path, key); !errors.Is(err, ErrAlreadyEncrypted) {
		t.Fatalf("already encrypted migration error = %v, want %v", err, ErrAlreadyEncrypted)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("already encrypted file changed")
	}
}

func TestEncryptFileInPlaceFailsClosedForEncryptedFileWithWrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.bin")
	key := bytes.Repeat([]byte{0x31}, 32)
	wrongKey := bytes.Repeat([]byte{0x32}, 32)
	plaintext := []byte("do not double encrypt")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	err = EncryptFileInPlace(path, wrongKey)
	if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wrong-key migration error = %v, want authentication failure", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("wrong-key migration mutated encrypted file")
	}
}
