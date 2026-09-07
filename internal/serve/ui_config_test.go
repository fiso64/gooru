package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestConfigValidatesGridSize(t *testing.T) {
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	if cfg.UI.GridSize != DefaultGridSize {
		t.Fatalf("default grid size = %d, want %d", cfg.UI.GridSize, DefaultGridSize)
	}
	cfg.UI.GridSize = 240
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid grid size rejected: %v", err)
	}
	cfg.UI.GridSize = MinGridSize - 1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.grid_size") {
		t.Fatalf("expected invalid grid size error, got %v", err)
	}
}

func TestConfigValidatesGridType(t *testing.T) {
	for _, gridType := range []string{"square", "fit", "tile"} {
		cfg := DefaultConfig(t.TempDir() + "/gooru.db")
		cfg.UI.GridType = gridType
		if err := cfg.Validate(); err != nil {
			t.Fatalf("valid grid type %q rejected: %v", gridType, err)
		}
	}
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.UI.GridType = "columns"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.grid_type") {
		t.Fatalf("expected invalid grid type error, got %v", err)
	}
}

func TestConfigLoadsGridSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  grid_size: 240\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.UI.GridSize != 240 {
		t.Fatalf("ui.grid_size = %d, want 240", cfg.UI.GridSize)
	}
}

func TestConfigLoadsGridType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  grid_type: tile\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.UI.GridType != "tile" {
		t.Fatalf("ui.grid_type = %q, want tile", cfg.UI.GridType)
	}
}

func TestConfigLoadsFullMediaDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  load_full_media_by_default: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.UI.LoadFullMediaByDefault {
		t.Fatal("ui.load_full_media_by_default was not loaded")
	}
}

func TestConfigLoadsFullscreenMediaDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  fullscreen_media_by_default: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.UI.FullscreenMediaByDefault {
		t.Fatal("ui.fullscreen_media_by_default was not loaded")
	}
}

func TestConfigRejectsOldMediaFullDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("media:\n  load_full_by_default: true\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "load_full_by_default") {
		t.Fatalf("expected old media.load_full_by_default to be rejected, got %v", err)
	}
}

func TestUIConfigIsPublicAndContainsRuntimePreferences(t *testing.T) {
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.UI.AccentColor = "#2f80ed"
	cfg.UI.LoadFullMediaByDefault = true
	cfg.UI.FullscreenMediaByDefault = true
	cfg.UI.ViewerFitMode = "original_size_if_fit"
	cfg.UI.ViewerActualSizeFitCap = false
	cfg.UI.ViewerScaling = "nearest"
	cfg.Media.ThumbnailSizes = []int{128, 384, 768}
	cfg.UI.GridSize = 240
	cfg.UI.GridType = "fit"
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
	if !strings.Contains(rec.Body.String(), `"fullscreen_media_by_default":true`) {
		t.Fatalf("response missing fullscreen-media preference: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"viewer_fit_mode":"original_size_if_fit"`) {
		t.Fatalf("response missing viewer fit mode: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"viewer_actual_size_fit_cap":false`) {
		t.Fatalf("response missing actual-size fit cap: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"viewer_scaling":"nearest"`) {
		t.Fatalf("response missing viewer scaling: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"grid_size":240`) {
		t.Fatalf("response missing grid size: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"grid_type":"fit"`) {
		t.Fatalf("response missing grid type: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"thumbnail_sizes":[128,384,768]`) {
		t.Fatalf("response missing thumbnail sizes: %s", rec.Body.String())
	}
}

func TestConfigLoadsViewerActualSizeFitCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  viewer_actual_size_fit_cap: false\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.UI.ViewerActualSizeFitCap {
		t.Fatal("ui.viewer_actual_size_fit_cap should allow explicitly disabling the default cap")
	}
	defaults := DefaultConfig(filepath.Join(dir, "default.db"))
	if !defaults.UI.ViewerActualSizeFitCap {
		t.Fatal("ui.viewer_actual_size_fit_cap should default enabled")
	}
}

func TestConfigLoadsAndValidatesViewerFitMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gooru.yaml")
	if err := os.WriteFile(path, []byte("ui:\n  viewer_fit_mode: FIT_DOWN_ONLY\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.UI.ViewerFitMode != "fit_down_only" {
		t.Fatalf("ui.viewer_fit_mode = %q, want fit_down_only", cfg.UI.ViewerFitMode)
	}
	cfg.UI.ViewerFitMode = "stretch"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.viewer_fit_mode") {
		t.Fatalf("expected invalid viewer fit mode error, got %v", err)
	}
}

func TestViewerPresentationConfigAliasesAndScaling(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.UI.ViewerFitMode = "screen"
	cfg.UI.ViewerScaling = "NEAREST"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate presentation config: %v", err)
	}
	if cfg.UI.ViewerFitMode != "fit_window" {
		t.Fatalf("legacy screen alias normalized to %q", cfg.UI.ViewerFitMode)
	}
	if cfg.UI.ViewerScaling != "nearest" {
		t.Fatalf("viewer scaling normalized to %q", cfg.UI.ViewerScaling)
	}
	cfg.UI.ViewerFitMode = "actual"
	cfg.UI.ViewerScaling = "nearest"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("actual fit mode should be accepted: %v", err)
	}
	cfg.UI.ViewerScaling = "bicubic"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ui.viewer_scaling") {
		t.Fatalf("expected invalid viewer scaling error, got %v", err)
	}
}
