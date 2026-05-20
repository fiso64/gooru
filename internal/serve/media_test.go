package serve

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestContentRouteSupportsRanges(t *testing.T) {
	file := writeMediaFile(t, []byte("0123456789"))
	server := newMediaTestServer(t, types.FileInfo{ID: 1, Path: file, Hash: "hash-content", Size: 10})
	req := authedRequest(http.MethodGet, "/api/v1/files/"+EncodeFileID(1)+"/content")
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "2345" {
		t.Fatalf("unexpected range body %q", got)
	}
}

func TestMediaRoutesRequireBearerToken(t *testing.T) {
	file := writeMediaFile(t, []byte("0123456789"))
	server := newMediaTestServer(t, types.FileInfo{ID: 2, Path: file, Hash: "hash-content", Size: 10})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+EncodeFileID(2)+"/content", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON error response, got %q", got)
	}
}

func TestThumbnailRouteGeneratesAndCaches(t *testing.T) {
	imagePath := writePNGImage(t)
	server := newMediaTestServer(t, types.FileInfo{ID: 7, Path: imagePath, Hash: "hash-image", Size: 100})

	for i, wantCache := range []string{"miss", "hit"} {
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/api/v1/files/"+EncodeFileID(7)+"/thumbnail?size=16")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Gooru-Cache"); got != wantCache {
			t.Fatalf("request %d expected cache %q, got %q", i+1, wantCache, got)
		}
		if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
			t.Fatalf("expected image/jpeg, got %q", got)
		}
		if rec.Body.Len() == 0 {
			t.Fatal("empty thumbnail response")
		}
	}
}

func newMediaTestServer(t *testing.T, file types.FileInfo) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Token = "secret"
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "media-cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "jpeg"
	cfg.Media.PreviewSize = 32
	return NewServerWithLibrary(cfg, mediaLibrary{file: file})
}

func writeMediaFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "content.bin")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	return path
}

func writePNGImage(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 9), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		t.Fatalf("write png: %v", err)
	}
	return path
}

type mediaLibrary struct {
	file types.FileInfo
}

func (l mediaLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return []types.FileInfo{l.file}, nil
}

func (l mediaLibrary) GetFile(_ context.Context, id int64) (types.FileInfo, error) {
	if id != l.file.ID {
		return types.FileInfo{}, ErrNotFound
	}
	return l.file, nil
}

func (mediaLibrary) ListTags(_ context.Context, _ bool) ([]TagDTO, error) {
	return nil, nil
}
