package serve

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

type protectedVideoRouteThumbnailer struct {
	sourceCalls int
	pathCalls   int
}

func (t *protectedVideoRouteThumbnailer) BackendVersion() string { return "protected-video-route-test" }

func (t *protectedVideoRouteThumbnailer) Thumbnail(string, io.Writer, int, string) error {
	return errors.New("unexpected ordinary-path thumbnail call")
}

func (t *protectedVideoRouteThumbnailer) ThumbnailSource(string, io.ReadSeeker, io.Writer, int, string) error {
	t.sourceCalls++
	return errors.New("unexpected streaming-source thumbnail call")
}

func (t *protectedVideoRouteThumbnailer) ThumbnailVideoPath(string, io.Writer, int, string) error {
	t.pathCalls++
	return nil
}

func TestProtectedVideoContainersUseSharedSeekableSource(t *testing.T) {
	t.Parallel()

	for _, ext := range []string{".mp4", ".webm", ".mkv"} {
		ext := ext
		t.Run(ext, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			path := filepath.Join(root, "clip"+ext)
			if err := os.WriteFile(path, []byte("video-fixture"), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg := DefaultConfig(filepath.Join(root, "gooru.db"))
			service := NewMediaService(cfg)
			thumbnailer := &protectedVideoRouteThumbnailer{}
			service.thumbnailer = thumbnailer

			if err := generateThumbnailFromLogicalSource(service, types.FileInfo{Path: path, Hash: "video-route"}, io.Discard, 320, "jpeg"); err != nil {
				t.Fatal(err)
			}
			if thumbnailer.pathCalls != 1 {
				t.Fatalf("seekable-video calls = %d, want 1", thumbnailer.pathCalls)
			}
			if thumbnailer.sourceCalls != 0 {
				t.Fatalf("streaming-source calls = %d, want 0", thumbnailer.sourceCalls)
			}
		})
	}
}
