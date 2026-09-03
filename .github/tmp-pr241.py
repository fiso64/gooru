from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old in text:
        p.write_text(text.replace(old, new))
        return
    if new not in text:
        raise SystemExit(f"expected text not found in {path}")


replace(
    "internal/serve/config.go",
    '\tPreviewSize       int    `yaml:"preview_size"`\n\tLoadFullByDefault bool   `yaml:"load_full_by_default"`\n',
    '\tPreviewSize     int    `yaml:"preview_size"`\n',
)
replace(
    "internal/serve/config.go",
    'type UIConfig struct {\n\tAccentColor string `yaml:"accent_color"`\n\tFontStyle   string `yaml:"font_style"`\n\tGridSize    int    `yaml:"grid_size"`\n}',
    'type UIConfig struct {\n\tAccentColor            string `yaml:"accent_color"`\n\tFontStyle              string `yaml:"font_style"`\n\tGridSize               int    `yaml:"grid_size"`\n\tLoadFullMediaByDefault bool   `yaml:"load_full_media_by_default"`\n}',
)
replace(
    "internal/serve/ui_config.go",
    "LoadFullMediaByDefault: s.cfg.Media.LoadFullByDefault,",
    "LoadFullMediaByDefault: s.cfg.UI.LoadFullMediaByDefault,",
)

tests = Path("internal/serve/ui_config_test.go")
text = tests.read_text()
text = text.replace('[]byte("media:\\n  load_full_by_default: true\\n")', '[]byte("ui:\\n  load_full_media_by_default: true\\n")', 1)
text = text.replace("cfg.Media.LoadFullByDefault", "cfg.UI.LoadFullMediaByDefault")
text = text.replace("media.load_full_by_default was not loaded", "ui.load_full_media_by_default was not loaded")
marker = "func TestUIConfigIsPublicAndContainsRuntimePreferences(t *testing.T) {"
regression = '''func TestConfigRejectsOldMediaFullDefault(t *testing.T) {
\tdir := t.TempDir()
\tpath := filepath.Join(dir, "gooru.yaml")
\tif err := os.WriteFile(path, []byte("media:\\n  load_full_by_default: true\\n"), 0o600); err != nil {
\t\tt.Fatalf("write config: %v", err)
\t}
\t_, err := LoadConfig(path, filepath.Join(dir, "gooru.db"), Overrides{})
\tif err == nil || !strings.Contains(err.Error(), "load_full_by_default") {
\t\tt.Fatalf("expected old media.load_full_by_default to be rejected, got %v", err)
\t}
}

'''
if "func TestConfigRejectsOldMediaFullDefault" not in text:
    if marker not in text:
        raise SystemExit("ui config test insertion point not found")
    text = text.replace(marker, regression + marker)
tests.write_text(text)

docs = Path("docs/CONFIG.md")
text = docs.read_text()
text = text.replace('| `media.load_full_by_default` | `false` | Start image viewers on the original/full media instead of the derived preview when an original is available. The viewer button remains available to switch back to the preview for the current viewer session. |\n', "")
ui_row = '| `ui.load_full_media_by_default` | `false` | Start image viewers on the original/full media instead of the derived preview when an original is available. The viewer button remains available to switch back to the preview for the current viewer session. |\n'
grid_row = '| `ui.grid_size` | `180` | Minimum media-grid cell width in pixels. Must be between `64` and `1024`. The grid remains fluid: cells expand to fill each row rather than becoming fixed-width. |\n'
if ui_row not in text:
    if grid_row not in text:
        raise SystemExit("UI documentation insertion point not found")
    text = text.replace(grid_row, grid_row + ui_row)
text = text.replace("  preview_size: 1280\n  load_full_by_default: false\n", "  preview_size: 1280\n")
if "  load_full_media_by_default: false\n" not in text:
    text = text.replace("  grid_size: 180\n```", "  grid_size: 180\n  load_full_media_by_default: false\n```")
docs.write_text(text)
