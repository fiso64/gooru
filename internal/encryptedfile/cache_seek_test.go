package encryptedfile

import (
	"io"
	"testing"
)

func TestEncryptedFileSeekWorkingSetReusesDecryptedChunks(t *testing.T) {
	plaintext := testPlaintext()
	key := testKey(0x43)
	path := writeEncryptedFixture(t, plaintext, key)

	file, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	counted := &countingReadAtCloser{readAtCloser: file.file}
	file.file = counted
	buf := make([]byte, 32<<10)

	// Loopback HTTP range serving for protected seekable video repeatedly seeks
	// among a small set of container regions. Each miss decrypts/authenticates a
	// complete 1 MiB chunk, so revisiting those regions must hit a bounded cache.
	for _, off := range []int64{0, ChunkSize + 64, 2*ChunkSize + 64, 0, ChunkSize + 64, 2*ChunkSize + 64} {
		if _, err := file.Seek(off, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadFull(file, buf); err != nil {
			t.Fatal(err)
		}
	}

	if counted.reads != 3 {
		t.Fatalf("ciphertext reads across repeated three-chunk seek working set = %d, want 3", counted.reads)
	}
}