package encryptedfile

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func testKey(value byte) []byte {
	return bytes.Repeat([]byte{value}, 32)
}

func testPlaintext() []byte {
	size := 2*ChunkSize + 9137
	out := make([]byte, size)
	for i := range out {
		out[i] = byte((i*31 + 17) % 251)
	}
	return out
}

func writeEncryptedFixture(t *testing.T, plaintext []byte, key []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "original.bin")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEncryptedFileRoundTripAndRandomAccess(t *testing.T) {
	plaintext := testPlaintext()
	key := testKey(0x41)
	path := writeEncryptedFixture(t, plaintext, key)

	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, plaintext[ChunkSize-64:ChunkSize]) {
		t.Fatal("ciphertext contains a distinctive plaintext segment")
	}

	file, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Size() != int64(len(plaintext)) {
		t.Fatalf("Size() = %d, want %d", file.Size(), len(plaintext))
	}

	all, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(all, plaintext) {
		t.Fatal("sequential plaintext mismatch")
	}

	start := int64(ChunkSize - 37)
	buf := make([]byte, 211)
	n, err := file.ReadAt(buf, start)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(buf) || !bytes.Equal(buf, plaintext[start:start+int64(len(buf))]) {
		t.Fatal("cross-chunk ReadAt mismatch")
	}

	if _, err := file.Seek(-128, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(tail, plaintext[len(plaintext)-128:]) {
		t.Fatal("seek/read tail mismatch")
	}
}

func TestEncryptedFileRejectsWrongKeyBeforeReads(t *testing.T) {
	path := writeEncryptedFixture(t, []byte("protected original"), testKey(0x22))
	if _, err := Open(path, testKey(0x23)); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("Open() error = %v, want ErrAuthentication", err)
	}
}

func TestEncryptedFileDetectsHeaderAndChunkTampering(t *testing.T) {
	plaintext := testPlaintext()
	key := testKey(0x51)

	t.Run("header", func(t *testing.T) {
		path := writeEncryptedFixture(t, plaintext, key)
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		byteAt := []byte{0}
		if _, err := file.ReadAt(byteAt, 20); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		byteAt[0] ^= 0x80
		if _, err := file.WriteAt(byteAt, 20); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		_ = file.Close()
		if _, err := Open(path, key); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("Open() error = %v, want ErrAuthentication", err)
		}
	})

	t.Run("chunk", func(t *testing.T) {
		path := writeEncryptedFixture(t, plaintext, key)
		file, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		offset := int64(headerSize + 123)
		byteAt := []byte{0}
		if _, err := file.ReadAt(byteAt, offset); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		byteAt[0] ^= 0x40
		if _, err := file.WriteAt(byteAt, offset); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		_ = file.Close()

		opened, err := Open(path, key)
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		buf := make([]byte, 256)
		if _, err := opened.ReadAt(buf, 0); !errors.Is(err, ErrAuthentication) {
			t.Fatalf("ReadAt() error = %v, want ErrAuthentication", err)
		}
	})
}

func TestEncryptedFileRejectsTruncationAndSizeMismatch(t *testing.T) {
	key := testKey(0x66)
	plaintext := []byte("exact-size payload")

	var encrypted bytes.Buffer
	if err := Encrypt(&encrypted, bytes.NewReader(plaintext), int64(len(plaintext)-1), key); !errors.Is(err, ErrPlaintextSize) {
		t.Fatalf("Encrypt() oversized source error = %v, want ErrPlaintextSize", err)
	}
	if err := Encrypt(io.Discard, bytes.NewReader(plaintext[:len(plaintext)-1]), int64(len(plaintext)), key); !errors.Is(err, ErrPlaintextSize) {
		t.Fatalf("Encrypt() short source error = %v, want ErrPlaintextSize", err)
	}

	path := writeEncryptedFixture(t, plaintext, key)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, key); !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("Open() truncated error = %v, want ErrInvalidFormat", err)
	}
}

func TestEncryptedEmptyFileRoundTrip(t *testing.T) {
	key := testKey(0x77)
	path := writeEncryptedFixture(t, nil, key)
	file, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if file.Size() != 0 {
		t.Fatalf("Size() = %d, want 0", file.Size())
	}
	buf := make([]byte, 1)
	if n, err := file.Read(buf); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("Read() = %d, %v; want 0, EOF", n, err)
	}
}
