package serve

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLogicalVideoSeekablePathSupportsRangesAndSanitizesName(t *testing.T) {
	plaintext := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	location, cleanup, available, err := logicalVideoSeekablePath("unsafe name?#.MP4", bytes.NewReader(plaintext), int64(len(plaintext)))
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

func TestLogicalVideoSeekablePathServesRangeWhileInitialReadIsBlocked(t *testing.T) {
	plaintext := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	source := &blockedInitialVideoReaderAt{
		data:         plaintext,
		firstStarted: make(chan struct{}),
		release:      make(chan struct{}),
	}
	defer source.unblock()

	location, cleanup, available, err := logicalVideoSeekablePath("clip.mp4", source, int64(len(plaintext)))
	if err != nil {
		t.Fatal(err)
	}
	if !available {
		t.Skip("loopback listener unavailable")
	}
	defer cleanup()

	initialDone := make(chan error, 1)
	go func() {
		resp, err := http.Get(location)
		if err != nil {
			initialDone <- err
			return
		}
		defer resp.Body.Close()
		_, err = io.Copy(io.Discard, resp.Body)
		initialDone <- err
	}()

	select {
	case <-source.firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("initial video response did not start reading")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=10-14")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("range request blocked behind initial response: %v", err)
	}
	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusPartialContent)
	}
	if got, want := string(body), string(plaintext[10:15]); got != want {
		t.Fatalf("range body = %q, want %q", got, want)
	}

	source.unblock()
	select {
	case err := <-initialDone:
		if err != nil {
			t.Fatalf("initial response failed after unblock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("initial response did not finish after unblock")
	}
}

type blockedInitialVideoReaderAt struct {
	data         []byte
	firstStarted chan struct{}
	release      chan struct{}
	startOnce    sync.Once
	releaseOnce  sync.Once
}

func (r *blockedInitialVideoReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off == 0 {
		r.startOnce.Do(func() { close(r.firstStarted) })
		<-r.release
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *blockedInitialVideoReaderAt) unblock() {
	r.releaseOnce.Do(func() { close(r.release) })
}
