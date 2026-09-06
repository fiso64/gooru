from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"missing replacement anchor in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))


replace(
    "internal/serve/config.go",
    '''type MediaConfig struct {
\tCacheDir        string `yaml:"cache_dir"`
\tThumbnailSizes  []int  `yaml:"thumbnail_sizes"`
\tThumbnailFormat string `yaml:"thumbnail_format"`
\tPreviewSize     int    `yaml:"preview_size"`
}''',
    '''type MediaConfig struct {
\tCacheDir           string `yaml:"cache_dir"`
\tThumbnailSizes     []int  `yaml:"thumbnail_sizes"`
\tThumbnailFormat    string `yaml:"thumbnail_format"`
\tPreviewSize        int    `yaml:"preview_size"`
\tPreviewEnabled     bool   `yaml:"preview_enabled"`
\tPreviewJPEGQuality int    `yaml:"preview_jpeg_quality"`
}''',
)
replace(
    "internal/serve/config.go",
    '''\t\tMedia: MediaConfig{
\t\t\tThumbnailSizes:  []int{256, 512},
\t\t\tThumbnailFormat: "jpeg",
\t\t\tPreviewSize:     1280,
\t\t},''',
    '''\t\tMedia: MediaConfig{
\t\t\tThumbnailSizes:     []int{256, 512},
\t\t\tThumbnailFormat:    "jpeg",
\t\t\tPreviewSize:        1280,
\t\t\tPreviewEnabled:     true,
\t\t\tPreviewJPEGQuality: derivativeJPEGQuality,
\t\t},''',
)
replace(
    "internal/serve/config.go",
    '''\tif cfg.Media.PreviewSize <= 0 {
\t\terrs = append(errs, errors.New("media.preview_size must be greater than zero"))
\t}
\tif cfg.Media.ThumbnailFormat != "jpeg" && cfg.Media.ThumbnailFormat != "png" {''',
    '''\tif cfg.Media.PreviewSize <= 0 {
\t\terrs = append(errs, errors.New("media.preview_size must be greater than zero"))
\t}
\tif cfg.Media.PreviewJPEGQuality < 1 || cfg.Media.PreviewJPEGQuality > 100 {
\t\terrs = append(errs, errors.New("media.preview_jpeg_quality must be between 1 and 100"))
\t}
\tif cfg.Media.ThumbnailFormat != "jpeg" && cfg.Media.ThumbnailFormat != "png" {''',
)

replace("internal/serve/ui_config.go", 'import "net/http"\n\ntype UIConfigResponse struct {', 'import "net/http"\n\nconst UICapabilityPreviewImages = "preview_images"\n\ntype UIConfigResponse struct {')
replace("internal/serve/ui_config.go", '\tOpaqueURLState           bool   `json:"opaque_url_state"`\n}', '\tOpaqueURLState           bool     `json:"opaque_url_state"`\n\tCapabilities             []string `json:"capabilities"`\n}')
replace("internal/serve/ui_config.go", 'func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {\n\twriteJSON(w, http.StatusOK, UIConfigResponse{', 'func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {\n\tcapabilities := make([]string, 0, 1)\n\tif s.cfg.Media.PreviewEnabled {\n\t\tcapabilities = append(capabilities, UICapabilityPreviewImages)\n\t}\n\twriteJSON(w, http.StatusOK, UIConfigResponse{')
replace("internal/serve/ui_config.go", '\t\tOpaqueURLState:           s.cfg.Encryption.Enabled && s.cfg.Encryption.OpaqueURLState,\n\t})', '\t\tOpaqueURLState:           s.cfg.Encryption.Enabled && s.cfg.Encryption.OpaqueURLState,\n\t\tCapabilities:             capabilities,\n\t})')

replace("internal/serve/media.go", 'import (\n\t"crypto/sha256"', 'import (\n\t"bytes"\n\t"crypto/sha256"')
replace("internal/serve/media.go", 'func (m *MediaService) ServeDerivative(w http.ResponseWriter, r *http.Request, file types.FileInfo, kind string) {\n\t// A static image derivative necessarily discards GIF animation.', 'func (m *MediaService) ServeDerivative(w http.ResponseWriter, r *http.Request, file types.FileInfo, kind string) {\n\tif kind == "preview" && !m.cfg.Media.PreviewEnabled {\n\t\tm.ServeContent(w, r, file)\n\t\treturn\n\t}\n\t// A static image derivative necessarily discards GIF animation.')
replace("internal/serve/media.go", '\tartifact, err := m.derivatives.GetOrGenerate(relativePath, func(dst io.Writer) error {\n\t\treturn m.generateThumbnail(file, dst, size, format)\n\t})', '\tartifact, err := m.derivatives.GetOrGenerate(relativePath, func(dst io.Writer) error {\n\t\treturn m.generateDerivative(file, dst, size, format, kind)\n\t})')
replace("internal/serve/media.go", 'func (m *MediaService) generateThumbnail(file types.FileInfo, dst io.Writer, size int, format string) error {', '''func (m *MediaService) generateDerivative(file types.FileInfo, dst io.Writer, size int, format string, kind string) error {
\tif kind == "preview" && format == "jpeg" && m.cfg.Media.PreviewJPEGQuality != derivativeJPEGQuality {
\t\tvar lossless bytes.Buffer
\t\tif err := m.generateThumbnail(file, &lossless, size, "png"); err != nil {
\t\t\treturn err
\t\t}
\t\timg, _, err := image.Decode(&lossless)
\t\tif err != nil {
\t\t\treturn fmt.Errorf("decode lossless preview intermediate: %w", err)
\t\t}
\t\treturn jpeg.Encode(dst, img, &jpeg.Options{Quality: m.cfg.Media.PreviewJPEGQuality})
\t}
\treturn m.generateThumbnail(file, dst, size, format)
}

func (m *MediaService) generateThumbnail(file types.FileInfo, dst io.Writer, size int, format string) error {''')
replace("internal/serve/media.go", '''\tkey := strings.Join([]string{
\t\tfile.Hash,
\t\tmediaKindForType(mediaTypeForPath(file.Path)),
\t\tkind,
\t\tstrconv.Itoa(size),
\t\tformat,
\t\tbackendVersion,
\t}, "|")''', '''\tqualityKey := ""
\tif kind == "preview" && format == "jpeg" {
\t\tqualityKey = strconv.Itoa(m.cfg.Media.PreviewJPEGQuality)
\t}
\tkey := strings.Join([]string{
\t\tfile.Hash,
\t\tmediaKindForType(mediaTypeForPath(file.Path)),
\t\tkind,
\t\tstrconv.Itoa(size),
\t\tformat,
\t\tqualityKey,
\t\tbackendVersion,
\t}, "|")''')

replace("frontend/src/lib/stores/runtimeConfig.ts", 'export type RuntimeConfig = {\n  loadFullMediaByDefault: boolean;', '''export const runtimeCapability = {
  previewImages: 'preview_images'
} as const;

export type RuntimeCapability = (typeof runtimeCapability)[keyof typeof runtimeCapability];

export type RuntimeConfig = {
  capabilities: string[];
  loadFullMediaByDefault: boolean;''')
replace("frontend/src/lib/stores/runtimeConfig.ts", 'export const runtimeConfig = writable<RuntimeConfig>({\n  loadFullMediaByDefault: false,', '''export function hasRuntimeCapability(config: RuntimeConfig, capability: RuntimeCapability): boolean {
  return config.capabilities.includes(capability);
}

export const runtimeConfig = writable<RuntimeConfig>({
  capabilities: [runtimeCapability.previewImages],
  loadFullMediaByDefault: false,''')

replace("frontend/src/lib/components/AppPage.svelte", '    opaque_url_state?: boolean;\n  };', '    opaque_url_state?: boolean;\n    capabilities?: string[];\n  };')
replace("frontend/src/lib/components/AppPage.svelte", '    runtimeConfig.set({\n      loadFullMediaByDefault: config.load_full_media_by_default ?? false,', '    runtimeConfig.set({\n      capabilities: Array.isArray(config.capabilities) ? config.capabilities : [\'preview_images\'],\n      loadFullMediaByDefault: config.load_full_media_by_default ?? false,')

replace("frontend/src/lib/components/PreviewDialog.svelte", "  import { runtimeConfig } from '$lib/stores/runtimeConfig';", "  import { hasRuntimeCapability, runtimeCapability, runtimeConfig } from '$lib/stores/runtimeConfig';")
replace("frontend/src/lib/components/PreviewDialog.svelte", '''  const originalAvailable = $derived(canUseOriginalInViewer(file));
  const comicAvailable = $derived(isComicFile(file));
  const currentComicPage = $derived(comicPageAt(comicManifest, comicPageIndex));
  const imageSource = $derived(comicEntered && currentComicPage ? currentComicPage.url : viewerImageSource(file, preferOriginal));''', '''  const originalAvailable = $derived(canUseOriginalInViewer(file));
  const previewAvailable = $derived(hasRuntimeCapability($runtimeConfig, runtimeCapability.previewImages));
  const effectivePreferOriginal = $derived(!previewAvailable || preferOriginal);
  const comicAvailable = $derived(isComicFile(file));
  const currentComicPage = $derived(comicPageAt(comicManifest, comicPageIndex));
  const imageSource = $derived(comicEntered && currentComicPage ? currentComicPage.url : viewerImageSource(file, effectivePreferOriginal));''')
replace("frontend/src/lib/components/PreviewDialog.svelte", '    void preloadViewerMediaSource(neighbor, viewerImageSource(neighbor, preferOriginal)).catch(() => undefined);', '    void preloadViewerMediaSource(neighbor, viewerImageSource(neighbor, effectivePreferOriginal)).catch(() => undefined);')
replace("frontend/src/lib/components/PreviewDialog.svelte", '  function toggleOriginalMedia() {\n    if (!originalAvailable) return;', '  function toggleOriginalMedia() {\n    if (!previewAvailable || !originalAvailable) return;')
replace("frontend/src/lib/components/PreviewDialog.svelte", "    if (key === 'q' && originalAvailable) {", "    if (key === 'q' && previewAvailable && originalAvailable) {")
replace("frontend/src/lib/components/PreviewDialog.svelte", '    {#if originalAvailable}\n      <button', '    {#if previewAvailable && originalAvailable}\n      <button')

replace("docs/CONFIG.md", '| `media.preview_size` | `1280` | Requested long-edge size for image previews. Must be greater than zero. |', '| `media.preview_size` | `1280` | Requested long-edge size for image previews. Must be greater than zero. |\n| `media.preview_enabled` | `true` | Generate and serve derived viewer previews. When disabled, preview requests fall back to original media and the WebUI treats original media as the only viewer source. Grid thumbnails remain enabled. |\n| `media.preview_jpeg_quality` | `92` | JPEG quality for generated viewer previews, from `1` to `100`. This does not change grid-thumbnail JPEG quality. |')
replace("docs/CONFIG.md", '  thumbnail_format: jpeg\n  preview_size: 1280', '  thumbnail_format: jpeg\n  preview_size: 1280\n  preview_enabled: true\n  preview_jpeg_quality: 92')

Path("internal/serve/preview_options_test.go").write_text(r'''package serve

import (
    "bytes"
    "image"
    "image/color"
    "image/jpeg"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "gooru.local/types"
)

func TestPreviewOptionsDefaultsAndValidation(t *testing.T) {
    cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
    if !cfg.Media.PreviewEnabled { t.Fatal("preview generation should default enabled") }
    if cfg.Media.PreviewJPEGQuality != 92 { t.Fatalf("preview quality = %d, want 92", cfg.Media.PreviewJPEGQuality) }
    cfg.Media.PreviewJPEGQuality = 0
    if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "media.preview_jpeg_quality") { t.Fatalf("expected preview quality validation error, got %v", err) }
    cfg.Media.PreviewJPEGQuality = 101
    if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "media.preview_jpeg_quality") { t.Fatalf("expected preview quality validation error, got %v", err) }
}

func TestUIConfigAdvertisesPreviewCapability(t *testing.T) {
    cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
    server := NewServer(cfg)
    rec := httptest.NewRecorder()
    server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil))
    if !strings.Contains(rec.Body.String(), `"capabilities":["preview_images"]`) { t.Fatalf("enabled response = %s", rec.Body.String()) }

    cfg.Media.PreviewEnabled = false
    server = NewServer(cfg)
    rec = httptest.NewRecorder()
    server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ui-config", nil))
    if !strings.Contains(rec.Body.String(), `"capabilities":[]`) { t.Fatalf("disabled response = %s", rec.Body.String()) }
}

func TestDisabledPreviewServesOriginalWithoutDerivative(t *testing.T) {
    dir := t.TempDir()
    src := filepath.Join(dir, "source.jpg")
    var encoded bytes.Buffer
    img := image.NewRGBA(image.Rect(0, 0, 4, 4))
    for y := 0; y < 4; y++ { for x := 0; x < 4; x++ { img.Set(x, y, color.RGBA{R: 180, G: 40, B: 20, A: 255}) } }
    if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil { t.Fatal(err) }
    if err := os.WriteFile(src, encoded.Bytes(), 0o600); err != nil { t.Fatal(err) }

    cfg := DefaultConfig(filepath.Join(dir, "gooru.db"))
    cfg.Media.PreviewEnabled = false
    cfg.Media.CacheDir = filepath.Join(dir, "cache")
    media := NewMediaService(cfg)
    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/preview", nil)
    media.ServeDerivative(rec, req, types.FileInfo{Path: src, Hash: "hash"}, "preview")
    if rec.Code != http.StatusOK { t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String()) }
    if !bytes.Equal(rec.Body.Bytes(), encoded.Bytes()) { t.Fatal("disabled preview did not return original bytes") }
    entries, err := os.ReadDir(cfg.Media.CacheDir)
    if err == nil && len(entries) != 0 { t.Fatalf("disabled preview created derivative cache entries: %v", entries) }
}
''')
