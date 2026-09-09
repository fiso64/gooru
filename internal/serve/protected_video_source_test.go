package serve

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLogicalVideoSeekablePathSupportsRangesAndSanitizesName(t *testing.T) {
	plaintext := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	location, cleanup, available, err := logicalVideoSeekablePath("unsafe name?#.MP4", bytes.NewReader(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skip("loopback listener unavailable")
	}
	defer cleanup()
	if !strings.HasSuffix(location, "/source.mp4") {
		t.Fatalf("location = %q, want sanitized source.mp4 suffix", location)
	}

	req, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=10-14")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusPartialContent)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(body), string(plaintext[10:15]); got != want {
		t.Fatalf("range body = %q, want %q", got, want)
	}
}
