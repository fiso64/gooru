package encryptedfile

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type shortReader struct {
	data []byte
	step int
}

func (r *shortReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := r.step
	if n <= 0 || n > len(r.data) {
		n = len(r.data)
	}
	if n > len(p) {
		n = len(p)
	}
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func TestEncryptStreamProducesStandardEncryptedFile(t *testing.T) {
	key := testKey(0x35)
	cases := []struct {
		name string
		size int
	}{
		{name: "empty", size: 0},
		{name: "small", size: 113},
		{name: "exact_chunk", size: ChunkSize},
		{name: "multi_chunk", size: 2*ChunkSize + 9137},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plaintext := make([]byte, tc.size)
			for i := range plaintext {
				plaintext[i] = byte((i*19 + 7) % 251)
			}
			path := filepath.Join(t.TempDir(), "stream.enc")
			out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			gotSize, err := EncryptStream(out, &shortReader{data: append([]byte(nil), plaintext...), step: 7919}, key)
			if closeErr := out.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
			if err != nil {
				t.Fatalf("EncryptStream: %v", err)
			}
			if gotSize != int64(len(plaintext)) {
				t.Fatalf("plaintext size = %d, want %d", gotSize, len(plaintext))
			}

			opened, err := Open(path, key)
			if err != nil {
				t.Fatalf("Open streamed file: %v", err)
			}
			decrypted, err := io.ReadAll(opened)
			if closeErr := opened.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
			if err != nil {
				t.Fatalf("read streamed file: %v", err)
			}
			if !bytes.Equal(decrypted, plaintext) {
				t.Fatal("streamed plaintext round-trip mismatch")
			}

			stored, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			wantStoredSize := int64(headerSize) + int64(len(plaintext)) + int64(chunkCount(int64(len(plaintext))))*16
			if int64(len(stored)) != wantStoredSize {
				t.Fatalf("stored size = %d, want %d", len(stored), wantStoredSize)
			}
			if len(plaintext) >= 64 && bytes.Contains(stored, plaintext[len(plaintext)/2:len(plaintext)/2+64]) {
				t.Fatal("streamed ciphertext contains a distinctive plaintext segment")
			}
		})
	}
}

func TestEncryptStreamSupportsRandomAccessAcrossChunkBoundary(t *testing.T) {
	key := testKey(0x62)
	plaintext := testPlaintext()
	path := filepath.Join(t.TempDir(), "stream.enc")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncryptStream(out, bytes.NewReader(plaintext), key); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}

	opened, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	start := int64(ChunkSize - 29)
	buf := make([]byte, 173)
	if _, err := opened.ReadAt(buf, start); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, plaintext[start:start+int64(len(buf))]) {
		t.Fatal("random access mismatch across streamed chunk boundary")
	}
}
