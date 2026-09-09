package serve

import (
	"path/filepath"
	"testing"
)

func TestLoadConfigBooruStyleDefaultsGridTypeToFit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "ui:\n  theme: booru-style\n")

	cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig booru-style defaults: %v", err)
	}
	if cfg.UI.GridType != "fit" {
		t.Fatalf("booru-style omitted grid_type=%q want fit", cfg.UI.GridType)
	}
	if cfg.UI.GridSize != DefaultGridSize {
		t.Fatalf("booru-style omitted grid_size=%d want %d", cfg.UI.GridSize, DefaultGridSize)
	}
}

func TestLoadConfigBooruStyleExplicitGridConfigWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "ui:\n  theme: booru-style\n  grid_type: square\n  grid_size: 320\n")

	cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig booru-style explicit grid: %v", err)
	}
	if cfg.UI.GridType != "square" {
		t.Fatalf("booru-style explicit grid_type=%q want square", cfg.UI.GridType)
	}
	if cfg.UI.GridSize != 320 {
		t.Fatalf("booru-style explicit grid_size=%d want 320", cfg.UI.GridSize)
	}
}

func TestLoadConfigDefaultThemeKeepsSquareGridDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "ui:\n  theme: default\n")

	cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig default theme defaults: %v", err)
	}
	if cfg.UI.GridType != DefaultGridType {
		t.Fatalf("default theme omitted grid_type=%q want %q", cfg.UI.GridType, DefaultGridType)
	}
}
