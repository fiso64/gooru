package serve

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigBooruThemesUseThemeDefaultsWhenOmitted(t *testing.T) {
	for _, theme := range []string{"booru-light", "booru-dark"} {
		t.Run(theme, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "serve.yaml")
			writeConfig(t, path, "ui:\n  theme: "+theme+"\n")

			cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
			if err != nil {
				t.Fatalf("LoadConfig %s defaults: %v", theme, err)
			}
			if cfg.UI.GridType != "fit" {
				t.Fatalf("%s omitted grid_type=%q want fit", theme, cfg.UI.GridType)
			}
			if cfg.UI.GridSize != 180 {
				t.Fatalf("%s omitted grid_size=%d want 180", theme, cfg.UI.GridSize)
			}
			if cfg.UI.PaginationMode != "paged" {
				t.Fatalf("%s omitted pagination_mode=%q want paged", theme, cfg.UI.PaginationMode)
			}
		})
	}
}

func TestLoadConfigBooruExplicitGridAndPaginationConfigWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "ui:\n  theme: booru-dark\n  grid_type: square\n  grid_size: 320\n  pagination_mode: infinite\n")

	cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig booru explicit config: %v", err)
	}
	if cfg.UI.GridType != "square" {
		t.Fatalf("booru explicit grid_type=%q want square", cfg.UI.GridType)
	}
	if cfg.UI.GridSize != 320 {
		t.Fatalf("booru explicit grid_size=%d want 320", cfg.UI.GridSize)
	}
	if cfg.UI.PaginationMode != "infinite" {
		t.Fatalf("booru explicit pagination_mode=%q want infinite", cfg.UI.PaginationMode)
	}
}

func TestLoadConfigDefaultThemeKeepsStandardDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "ui:\n  theme: default\n")

	cfg, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig default theme defaults: %v", err)
	}
	if cfg.UI.GridType != DefaultGridType {
		t.Fatalf("default theme omitted grid_type=%q want %q", cfg.UI.GridType, DefaultGridType)
	}
	if cfg.UI.GridSize != DefaultGridSize {
		t.Fatalf("default theme omitted grid_size=%d want %d", cfg.UI.GridSize, DefaultGridSize)
	}
	if cfg.UI.PaginationMode != DefaultPaginationMode {
		t.Fatalf("default theme omitted pagination_mode=%q want %q", cfg.UI.PaginationMode, DefaultPaginationMode)
	}
}

func TestLoadConfigBooruStillRejectsUnknownUIFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serve.yaml")
	writeConfig(t, path, "ui:\n  theme: booru-light\n  unknown_grid_setting: true\n")

	_, err := LoadConfig(path, filepath.Join(t.TempDir(), "gooru.db"), Overrides{})
	if err == nil || !strings.Contains(err.Error(), "unknown_grid_setting") {
		t.Fatalf("expected unknown ui field rejection, got %v", err)
	}
}
