package filesource

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
)

func TestProtectedSourceHTTPRangeCrossesEncryptedChunkBoundary(t *testing.T) {
	plaintext := benchmarkPlaintext(2*encryptedfile.ChunkSize + 4096)
	resolver, path := protectedTestFixture(t, plaintext)

	source, err := resolver.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	const beforeBoundary = 37
	const afterBoundary = 83
	start := int64(encryptedfile.ChunkSize - beforeBoundary)
	end := int64(encryptedfile.ChunkSize + afterBoundary - 1)

	req := httptest.NewRequest(http.MethodGet, "http://gooru.test/media", nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	rec := httptest.NewRecorder()

	http.ServeContent(rec, req, "media.mp4", source.ModTime(), source)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusPartialContent, rec.Body.String())
	}
	want := plaintext[start : end+1]
	if got := rec.Body.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("range plaintext mismatch: got %d bytes, want %d", len(got), len(want))
	}
	wantRange := fmt.Sprintf("bytes %d-%d/%d", start, end, len(plaintext))
	if got := rec.Header().Get("Content-Range"); got != wantRange {
		t.Fatalf("Content-Range = %q, want %q", got, wantRange)
	}
	if got := rec.Header().Get("Accept-Ranges"); got != "bytes" {
		t.Fatalf("Accept-Ranges = %q, want bytes", got)
	}
	if got := rec.Header().Get("Content-Length"); got != fmt.Sprint(len(want)) {
		t.Fatalf("Content-Length = %q, want %d", got, len(want))
	}
}

func protectedTestFixture(t *testing.T, plaintext []byte) (*Resolver, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "media.bin")
	key := bytes.Repeat([]byte{0xa7}, 32)
	writeEncryptedTestFile(t, path, plaintext, key)
	resolver, err := NewProtected(key, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	return resolver, path
}
