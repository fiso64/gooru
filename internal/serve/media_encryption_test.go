package serve

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func protectedMediaTestServer(t *testing.T, file types.FileInfo) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = make([]byte, securekey.Size)
	cfg.Media.CacheDir = filepath.Join(t.TempDir(), "media-cache")
	cfg.Media.ThumbnailSizes = []int{16}
	cfg.Media.ThumbnailFormat = "jpeg"
	cfg.Media.PreviewSize = 32
	return NewServerWithLibrary(cfg, mediaLibrary{file: file})
}

func TestProtectedContentDisablesBrowserCaching(t *testing.T) {
	file := writeNamedMediaFile(t, "photo.png", []byte("png"))
	server := protectedMediaTestServer(t, types.FileInfo{ID: 91, Path: file, Hash: "protected-content", Size: 3})
	encryptMediaFixture(t, file, server.cfg.Encryption.Key)
	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(91)+"/content")

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
	if got := rec.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
}

func TestProtectedThumbnailUsesEncryptedPersistentCache(t *testing.T) {
	imagePath := writePNGImage(t)
	server := protectedMediaTestServer(t, types.FileInfo{ID: 92, Path: imagePath, Hash: "protected-thumbnail", Size: 100})
	encryptMediaFixture(t, imagePath, server.cfg.Encryption.Key)

	for i, want := range []string{"miss", "hit"} {
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(92)+"/thumbnail?size=16")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Gooru-Cache"); got != want {
			t.Fatalf("request %d cache = %q, want %q", i+1, got, want)
		}
		if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("request %d Cache-Control = %q", i+1, got)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("request %d returned an empty derivative", i+1)
		}
	}
}

func TestOrdinaryThumbnailStillUsesPersistentCache(t *testing.T) {
	imagePath := writePNGImage(t)
	server := newMediaTestServer(t, types.FileInfo{ID: 93, Path: imagePath, Hash: "ordinary-thumbnail", Size: 100})
	for i, want := range []string{"miss", "hit"} {
		rec := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(93)+"/thumbnail?size=16")
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("X-Gooru-Cache") != want {
			t.Fatalf("request %d status/cache = %d/%q, want 200/%q", i+1, rec.Code, rec.Header().Get("X-Gooru-Cache"), want)
		}
		if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Fatalf("ordinary cache policy changed: %q", got)
		}
	}
}
