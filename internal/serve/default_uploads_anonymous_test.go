package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUnauthenticatedDefaultConfigRejectsUploads(t *testing.T) {
	root := testDefaultUploadRoot(t)
	configPath := filepath.Join(t.TempDir(), "serve.yaml")
	if err := os.WriteFile(configPath, []byte("auth:\n  enabled: false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Uploads.Enabled || len(cfg.Uploads.Targets) != 0 {
		t.Fatalf("anonymous instance received uploads: %+v", cfg.Uploads)
	}
	if _, err := os.Stat(filepath.Join(root, "uploads")); !os.IsNotExist(err) {
		t.Fatalf("anonymous instance created an upload directory: %v", err)
	}
	server := NewServerWithLibrary(cfg, &recordingUploadLibrary{})
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, uploadRequest(t, map[string]string{"private.txt": "hello"}, nil))
	assertAPIError(t, response, http.StatusForbidden, "uploads_disabled")
}
