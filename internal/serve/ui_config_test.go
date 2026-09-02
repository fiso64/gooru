package serve

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigValidatesAccentColor(t *testing.T) {
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.UI.AccentColor = "#2f80ed"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid accent color rejected: %v", err)
	}
	if cfg.UI.AccentColor != "#2f80ed" {
		t.Fatalf("accent color normalized unexpectedly: %q", cfg.UI.AccentColor)
	}

	cfg.UI.AccentColor = "red; background:url(https://example.invalid)"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.accent_color") {
		t.Fatalf("expected invalid accent color error, got %v", err)
	}
}

func TestUIConfigIsPublicAndContainsRuntimePreferences(t *testing.T) {
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.UI.AccentColor = "#2f80ed"
	cfg.Media.LoadFullByDefault = true
	server := NewServer(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil)
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"accent_color":"#2f80ed"`) {
		t.Fatalf("response missing accent: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"load_full_media_by_default":true`) {
		t.Fatalf("response missing full-media preference: %s", rec.Body.String())
	}
}
