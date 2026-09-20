package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnauthenticatedExplicitUploadOptIn(t *testing.T) {
	root := testDefaultUploadRoot(t)
	configPath := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, configPath, "auth:\n  enabled: false\nuploads:\n  enabled: true\n")
	cfg, err := LoadConfig(configPath, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Uploads.Enabled || len(cfg.Uploads.Targets) != 1 {
		t.Fatalf("explicit unauthenticated opt-in lost its default target: %+v", cfg.Uploads)
	}
	want := filepath.Join(root, "uploads")
	if runtime.GOOS == "windows" {
		want = filepath.Join(root, "Gooru", "uploads")
	}
	if cfg.Uploads.Targets[0].Path != want {
		t.Fatalf("unexpected upload target %q rather than %q", cfg.Uploads.Targets[0].Path, want)
	}
	srv := NewServerWithLibrary(cfg, &recordingUploadLibrary{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"anonymous.txt": "opted in"}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("explicit opt-in upload returned %d: %s", rec.Code, rec.Body.String())
	}
	got, err := os.ReadFile(filepath.Join(cfg.Uploads.Targets[0].Path, "anonymous.txt"))
	if err != nil || string(got) != "opted in" {
		t.Fatalf("anonymous upload was not persisted: %q (%v)", got, err)
	}
}
