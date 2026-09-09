package serve

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

type protectedWebMRouteThumbnailer struct {
	sourceCalls int
	pathCalls   int
}

func (t *protectedWebMRouteThumbnailer) BackendVersion() string { return "protected-webm-route-test" }

func (t *protectedWebMRouteThumbnailer) Thumbnail(string, io.Writer, int, string) error {
	return errors.New("unexpected ordinary-path thumbnail call")
}

func (t *protectedWebMRouteThumbnailer) ThumbnailSource(_ string, src io.ReadSeeker, dst io.Writer, _ int, _ string) error {
	t.sourceCalls++
	_, err := io.Copy(dst, src)
	return err
}

func (t *protectedWebMRouteThumbnailer) ThumbnailVideoPath(string, io.Writer, int, string) error {
	t.pathCalls++
	return errors.New("unexpected seekable-video thumbnail call")
}

func TestProtectedWebMThumbnailUsesLogicalSourceInsteadOfSeekableLoopback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "clip.webm")
	plaintext := []byte("streamable-webm-fixture")
	if err := os.WriteFile(path, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig(filepath.Join(root, "gooru.db"))
	service := NewMediaService(cfg)
	thumbnailer := &protectedWebMRouteThumbnailer{}
	service.thumbnailer = thumbnailer

	var dst bytes.Buffer
	if err := generateThumbnailFromLogicalSource(service, types.FileInfo{Path: path, Hash: "webm-route"}, &dst, 320, "jpeg"); err != nil {
		t.Fatal(err)
	}
	if thumbnailer.sourceCalls != 1 {
		t.Fatalf("logical-source calls = %d, want 1", thumbnailer.sourceCalls)
	}
	if thumbnailer.pathCalls != 0 {
		t.Fatalf("seekable-video calls = %d, want 0", thumbnailer.pathCalls)
	}
	if got := dst.Bytes(); !bytes.Equal(got, plaintext) {
		t.Fatalf("logical-source bytes = %q, want %q", got, plaintext)
	}
}
