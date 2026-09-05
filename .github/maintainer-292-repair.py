from pathlib import Path

config = Path('internal/serve/config.go')
text = config.read_text()
text = text.replace(
    'UIConfig{FontStyle: "editorial", GridSize: DefaultGridSize, GridType: DefaultGridType}',
    'UIConfig{FontStyle: "editorial", GridSize: DefaultGridSize, GridType: DefaultGridType, ViewerFitMode: "fit_window", ViewerScaling: "smooth"}',
    1,
)
grid_validation = '''\tcfg.UI.GridType = strings.ToLower(strings.TrimSpace(cfg.UI.GridType))
\tif cfg.UI.GridType == "" {
\t\tcfg.UI.GridType = DefaultGridType
\t}
\tswitch cfg.UI.GridType {
\tcase "square", "fit", "tile":
\tdefault:
\t\terrs = append(errs, errors.New("ui.grid_type must be one of: square, fit, tile"))
\t}
'''
viewer_validation = '''\tcfg.UI.ViewerFitMode = strings.ToLower(strings.TrimSpace(cfg.UI.ViewerFitMode))
\tif cfg.UI.ViewerFitMode == "" {
\t\tcfg.UI.ViewerFitMode = "fit_window"
\t}
\tif cfg.UI.ViewerFitMode == "screen" {
\t\tcfg.UI.ViewerFitMode = "fit_window"
\t}
\tswitch cfg.UI.ViewerFitMode {
\tcase "fit_window", "fit_down_only", "original_size_if_fit", "actual":
\tdefault:
\t\terrs = append(errs, errors.New("ui.viewer_fit_mode must be one of: fit_window, fit_down_only, original_size_if_fit, actual"))
\t}
\tcfg.UI.ViewerScaling = strings.ToLower(strings.TrimSpace(cfg.UI.ViewerScaling))
\tif cfg.UI.ViewerScaling == "" {
\t\tcfg.UI.ViewerScaling = "smooth"
\t}
\tswitch cfg.UI.ViewerScaling {
\tcase "smooth", "nearest":
\tdefault:
\t\terrs = append(errs, errors.New("ui.viewer_scaling must be one of: smooth, nearest"))
\t}
'''
if viewer_validation not in text:
    if grid_validation not in text:
        raise SystemExit('grid validation anchor missing')
    text = text.replace(grid_validation, grid_validation + viewer_validation, 1)
config.write_text(text)

docs = Path('docs/CONFIG.md')
text = docs.read_text()
grid_row = '| `ui.grid_type` | `square` | Gallery layout: `square` keeps the existing cropped square grid, `fit` keeps square cells but contains the whole image with transparent surrounding space, and `tile` uses justified non-square aspect-preserving rows. All modes keep a bounded virtual DOM for large libraries. |\n'
viewer_rows = '| `ui.viewer_fit_mode` | `fit_window` | Default viewer fit policy: `fit_window` (legacy alias `screen`) fills the available viewer bounds, `fit_down_only` never enlarges smaller media, `original_size_if_fit` keeps media at 1:1 when it fits and otherwise scales down, and `actual` starts at original size. Press `V` to cycle the fit policies for the current browser session. |\n| `ui.viewer_scaling` | `smooth` | Browser-side image interpolation: `smooth` uses normal browser filtering and `nearest` uses nearest-neighbor/pixelated scaling. Press `S` in the viewer to toggle it for the current browser session. |\n'
if '`ui.viewer_fit_mode`' not in text:
    if grid_row not in text:
        raise SystemExit('grid docs anchor missing')
    text = text.replace(grid_row, grid_row + viewer_rows, 1)
sample = '  grid_size: 200\n  grid_type: square\n'
if '  viewer_fit_mode: fit_window\n' not in text:
    if sample not in text:
        raise SystemExit('ui sample anchor missing')
    text = text.replace(sample, sample + '  viewer_fit_mode: fit_window\n  viewer_scaling: smooth\n', 1)
docs.write_text(text)
