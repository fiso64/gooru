package serve

import (
  "image/color"
  "net/http"
  "path/filepath"
  "testing"

  "gooru.local/types"
)

// Page URLs do not carry their parent archive's version. Browser caches
// must re-request them even when the encrypted/unencrypted server cache hits.
func TestOrdinaryComicPageVariantDoesNotBecomeBrowserImmutable(t *testing.T) {
  path := writeComic(t, map[string][]byte{"01.png": tinyPNG(t, 48, 24, color.White)})
  cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
  cfg.Media.CacheDir = t.TempDir()
  service := NewMediaService(cfg)
  file := types.FileInfo{Path: path, Hash: "versioned-archive"}
  target := "/api/v1/comics/book/0?page=0&variant=preview"
  for _, status := range []string{"miss", "hit"} {
    rec := comicRequest(t, service, file, target)
    if rec.Code != http.StatusOK {
      t.Fatalf("variant status: %d: %s", rec.Code, rec.Body.String())
    }
    if rec.Header().Get("X-Gooru-Cache") != status {
      t.Fatalf("cache status = %q, want %q", rec.Header().Get("X-Gooru-Cache"), status)
    }
    if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
      t.Fatalf("mutable variant may be cached by browser: %q", got)
    }
  }
}
