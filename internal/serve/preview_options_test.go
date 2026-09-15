package serve

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/types"
)

func TestPreviewOptionsDefaultsAndValidation(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if !cfg.Media.PreviewEnabled {
		t.Fatal("preview generation should default enabled")
	}
	if cfg.Media.PreviewJPEGQuality != 92 {
		t.Fatalf("preview quality = %d, want 92", cfg.Media.PreviewJPEGQuality)
	}
	cfg.Media.PreviewJPEGQuality = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "media.preview_jpeg_quality") {
		t.Fatalf("expected preview quality validation error, got %v", err)
	}
	cfg.Media.PreviewJPEGQuality = 101
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "media.preview_jpeg_quality") {
		t.Fatalf("expected preview quality validation error, got %v", err)
	}
}

func TestUIConfigAdvertisesPreviewCapability(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil))
	if !strings.Contains(rec.Body.String(), `"capabilities":["preview_images"]`) {
		t.Fatalf("enabled response = %s", rec.Body.String())
	}

	cfg.Media.PreviewEnabled = false
	server = NewServer(cfg)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil))
	if !strings.Contains(rec.Body.String(), `"capabilities":[]`) {
		t.Fatalf("disabled response = %s", rec.Body.String())
	}
}

func TestDisabledPreviewServesOriginalWithoutDerivative(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.jpg")
	var encoded bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 180, G: 40, B: 20, A: 255})
		}
	}
	if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Media.PreviewEnabled = false
	cfg.Media.CacheDir = filepath.Join(dir, "cache")
	media := NewMediaService(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/preview", nil)
	media.ServeDerivative(rec, req, types.FileInfo{Path: src, Hash: "hash"}, "preview")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(rec.Body.Bytes(), encoded.Bytes()) {
		t.Fatal("disabled preview did not return original bytes")
	}
	entries, err := os.ReadDir(cfg.Media.CacheDir)
	if err == nil && len(entries) != 0 {
		t.Fatalf("disabled preview created derivative cache entries: %v", entries)
	}
}

func TestDisabledPreviewResolverFailureReturnsServiceUnavailable(t *testing.T) {
	file := types.FileInfo{
		ID:   63,
		Path: filepath.Join(t.TempDir(), "source.jpg"),
		Hash: "preview-resolver-failure",
	}
	server := newMediaTestServer(t, file)
	server.media.cfg.Media.PreviewEnabled = false
	server.media.sourceResolverErr = errors.New("resolver initialization failed")

	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(file.ID)+"/preview")
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected resolver failure to return 503, got %d: %s", rec.Code, rec.Body.String())
	}
}
