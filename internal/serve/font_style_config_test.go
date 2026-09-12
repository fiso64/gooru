package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestFontStyleConfigDefaultsAndNormalizes(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.UI.FontStyle != "comic" {
		t.Fatalf("default font style = %q, want comic", cfg.UI.FontStyle)
	}

	cfg.UI.FontStyle = " COMIC "
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate comic font style: %v", err)
	}
	if cfg.UI.FontStyle != "comic" {
		t.Fatalf("normalized font style = %q, want comic", cfg.UI.FontStyle)
	}
}

func TestFontStyleConfigRejectsUnknownPreset(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.UI.FontStyle = "papyrus"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.font_style must be one of: editorial, modern, comic") {
		t.Fatalf("expected font style validation error, got %v", err)
	}
}

func TestUIConfigExposesFontStyle(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.UI.FontStyle = "modern"
	server := NewServer(cfg)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil)

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("ui config status = %d: %s", recorder.Code, recorder.Body.String())
	}
	var response UIConfigResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.FontStyle != "modern" {
		t.Fatalf("font style = %q, want modern", response.FontStyle)
	}
}
