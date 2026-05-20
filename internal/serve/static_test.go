package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendServesIndexAndFallback(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.FrontendDir = writeFrontendBuild(t)
	server := NewServer(cfg)

	for _, target := range []string{"/", "/library/search"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, target, nil)
		server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s expected 200, got %d: %s", target, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s expected no-cache index response, got %q", target, got)
		}
	}
}

func TestFrontendImmutableAssetsUseLongCache(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.FrontendDir = writeFrontendBuild(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/_app/immutable/app.js", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache control %q", got)
	}
}

func writeFrontendBuild(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>Gooru</title>"), 0600); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assetDir := filepath.Join(root, "_app", "immutable")
	if err := os.MkdirAll(assetDir, 0700); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "app.js"), []byte("console.log('gooru')"), 0600); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	return root
}
