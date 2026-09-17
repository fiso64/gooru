package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		if got := rec.Header().Get("ETag"); got == "" {
			t.Fatalf("%s expected content ETag", target)
		}
		if got := rec.Header().Get("Last-Modified"); got != "" {
			t.Fatalf("%s index response should not expose an mtime validator, got %q", target, got)
		}
	}
}

func TestFrontendIndexETagTracksContentWithSameModTime(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.FrontendDir = writeFrontendBuildWithIndex(t, "<!doctype html><title>first</title>")
	indexPath := filepath.Join(cfg.Server.FrontendDir, "index.html")
	modTime := time.Unix(1700000000, 0)
	if err := os.Chtimes(indexPath, modTime, modTime); err != nil {
		t.Fatalf("set initial index mtime: %v", err)
	}
	server := NewServer(cfg)

	first := httptest.NewRecorder()
	server.Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("initial index expected 200, got %d: %s", first.Code, first.Body.String())
	}
	oldETag := first.Header().Get("ETag")
	if oldETag == "" {
		t.Fatal("initial index missing ETag")
	}

	const updated = "<!doctype html><title>second</title>"
	if err := os.WriteFile(indexPath, []byte(updated), 0600); err != nil {
		t.Fatalf("replace index: %v", err)
	}
	if err := os.Chtimes(indexPath, modTime, modTime); err != nil {
		t.Fatalf("restore index mtime: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", oldETag)
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("changed index with same mtime expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != updated {
		t.Fatalf("changed index body = %q, want %q", got, updated)
	}
	if got := rec.Header().Get("ETag"); got == "" || got == oldETag {
		t.Fatalf("changed index ETag = %q, old = %q", got, oldETag)
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

func TestFrontendIndexUsesStaticCSP(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.FrontendDir = writeFrontendBuildWithIndex(t, `<script>window.__gooru = true;</script><script src="/_app/immutable/app.js"></script>`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("index CSP missing static script policy, got %q", csp)
	}
	if strings.Contains(csp, "sha256-") {
		t.Fatalf("index CSP should not derive runtime script hashes, got %q", csp)
	}
}

func writeFrontendBuild(t *testing.T) string {
	return writeFrontendBuildWithIndex(t, "<!doctype html><title>Gooru</title>")
}

func writeFrontendBuildWithIndex(t *testing.T, index string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(index), 0600); err != nil {
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
