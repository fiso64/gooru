package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigLoadsAndValidatesUIThemes(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "light", input: "BOORU-LIGHT", want: "booru-light"},
		{name: "dark", input: "BOORU-DARK", want: "booru-dark"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "gooru.yaml")
			if err := os.WriteFile(path, []byte("ui:\n  theme: "+test.input+"\n"), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if cfg.UI.Theme != test.want {
				t.Fatalf("ui.theme = %q, want %q", cfg.UI.Theme, test.want)
			}
			if cfg.UI.FontStyleConfigured {
				t.Fatal("font_style should remain unconfigured when omitted")
			}
		})
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.UI.Theme = "booru-style"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.theme") {
		t.Fatalf("expected renamed booru-style theme to be rejected, got %v", err)
	}
}

func TestBooruFontStyleTracksExplicitConfiguration(t *testing.T) {
	for _, style := range []string{"editorial", "modern", "comic"} {
		t.Run(style, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "gooru.yaml")
			body := "ui:\n  theme: booru-light\n  font_style: " + style + "\n"
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			if !cfg.UI.FontStyleConfigured {
				t.Fatalf("font_style %q should be marked explicitly configured", style)
			}
			if cfg.UI.FontStyle != style {
				t.Fatalf("ui.font_style = %q, want %q", cfg.UI.FontStyle, style)
			}
		})
	}
}

func TestUIThemeDefaultsAndIsPublicRuntimeConfig(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	if cfg.UI.Theme != DefaultUITheme {
		t.Fatalf("default ui.theme = %q, want %q", cfg.UI.Theme, DefaultUITheme)
	}
	cfg.UI.Theme = "booru-dark"
	server := NewServer(cfg)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil)
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"ui_theme":"booru-dark"`) {
		t.Fatalf("response missing ui theme: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"font_style_configured":false`) {
		t.Fatalf("response missing omitted font-style state: %s", rec.Body.String())
	}

	cfg.UI.FontStyle = "modern"
	cfg.UI.FontStyleConfigured = true
	server = NewServer(cfg)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `"font_style":"modern"`) || !strings.Contains(rec.Body.String(), `"font_style_configured":true`) {
		t.Fatalf("response missing explicit font-style state: %s", rec.Body.String())
	}
}
