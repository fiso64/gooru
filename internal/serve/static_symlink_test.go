package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrontendDoesNotServeSymlinkOutsideStaticRoot(t *testing.T) {
	root := writeFrontendBuild(t)
	externalDir := t.TempDir()
	secretPath := filepath.Join(externalDir, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside-static-root-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretPath, filepath.Join(root, "leak.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.FrontendDir = root
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leak.txt", nil)
	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected escaped symlink to return 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "outside-static-root-secret") {
		t.Fatal("response disclosed file outside frontend root")
	}
}

func TestFrontendAllowsConfiguredRootSymlink(t *testing.T) {
	realRoot := writeFrontendBuild(t)
	linkParent := t.TempDir()
	linkedRoot := filepath.Join(linkParent, "frontend")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.FrontendDir = linkedRoot
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected symlinked frontend root to serve normally, got %d: %s", rec.Code, rec.Body.String())
	}
}
