package encryptedfile

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	buf := make([]byte, 4<<10)

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

func TestEncryptedFileHTTPRangeWorkingSetReusesDecryptedChunks(t *testing.T) {
	plaintext := testPlaintext()
	key := testKey(0x44)
	path := writeEncryptedFixture(t, plaintext, key)

	file, err := Open(path, key)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	counted := &countingReadAtCloser{readAtCloser: file.file}
	file.file = counted

	// Protected seek-oriented video is exposed to ffprobe/ffmpeg through a
	// short-lived loopback HTTP range source backed by this encrypted File. Test
	// that boundary directly rather than relying only on synthetic Seek/Read
	// calls: revisiting the same container working set must not re-read and
	// re-authenticate whole encrypted chunks.
	starts := []int64{0, ChunkSize + 64, 2*ChunkSize + 64, 0, ChunkSize + 64, 2*ChunkSize + 64}
	for _, start := range starts {
		end := start + (4 << 10) - 1
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/protected-video", nil)
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

		http.ServeContent(recorder, request, "protected.mp4", file.ModTime(), file)
		if recorder.Code != http.StatusPartialContent {
			t.Fatalf("range %d-%d status = %d, want %d", start, end, recorder.Code, http.StatusPartialContent)
		}
		if got := recorder.Body.Len(); got != 4<<10 {
			t.Fatalf("range %d-%d body length = %d, want %d", start, end, got, 4<<10)
		}
	}

	if counted.reads != 3 {
		t.Fatalf("ciphertext reads across repeated HTTP three-chunk range working set = %d, want 3", counted.reads)
	}
}
