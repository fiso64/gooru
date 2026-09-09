package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigLoadsAndValidatesUITheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  theme: BOORU-STYLE\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.UI.Theme != "booru-style" {
		t.Fatalf("ui.theme = %q, want booru-style", cfg.UI.Theme)
	}

	cfg.UI.Theme = "unknown"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.theme") {
		t.Fatalf("expected invalid ui.theme error, got %v", err)
	}
}

func TestUIThemeDefaultsAndIsPublicRuntimeConfig(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.UI.Theme != DefaultUITheme {
		t.Fatalf("default ui.theme = %q, want %q", cfg.UI.Theme, DefaultUITheme)
	}
	cfg.UI.Theme = "booru-style"
	server := NewServer(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil)
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ui_theme":"booru-style"`) {
		t.Fatalf("response missing ui theme: %s", rec.Body.String())
	}
}
