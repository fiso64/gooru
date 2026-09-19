package serve

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gooru.local/types"
)

func comicRequest(t *testing.T, service *MediaService, file types.FileInfo, target string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	service.ServeComic(response, httptest.NewRequest(http.MethodGet, target, nil), file, "book")
	return response
}

func comicManifestForTest(t *testing.T, response *httptest.ResponseRecorder) comicManifest {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("manifest status = %d: %s", response.Code, response.Body.String())
	}
	var manifest comicManifest
	if err := json.Unmarshal(response.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Pages) != 1 {
		t.Fatalf("manifest pages = %d, want 1", len(manifest.Pages))
	}
	return manifest
}

func TestComicPagePreviewOriginalAndDisabledFallback(t *testing.T) {
	original := tinyPNG(t, 48, 24, color.RGBA{R: 210, A: 255})
	path := writeComic(t, map[string][]byte{"01.png": original})
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = t.TempDir()
	cfg.Media.PreviewSize = 12
	cfg.Media.ThumbnailFormat = "png"
	service := NewMediaService(cfg)
	file := types.FileInfo{Path: path, Hash: "comic-preview"}
	manifest := comicManifestForTest(t, comicRequest(t, service, file, "/api/v1/comics/book"))
	page := manifest.Pages[0]
	if page.URL != "/api/v1/comics/book/0" || page.Preview != page.URL+"?variant=preview" || page.Lossless != "" {
		t.Fatalf("unexpected page variants: %+v", page)
	}
	for iteration, cacheStatus := range []string{"miss", "hit"} {
		preview := comicRequest(t, service, file, page.Preview+"\u0026page=0")
		if preview.Code != http.StatusOK || preview.Header().Get("X-Gooru-Cache") != cacheStatus {
			t.Fatalf("preview %d: status=%d cache=%q body=%s", iteration, preview.Code, preview.Header().Get("X-Gooru-Cache"), preview.Body.String())
		}
		decoded, _, err := image.Decode(bytes.NewReader(preview.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Bounds().Dx() >= 48 || decoded.Bounds().Dy() >= 24 {
			t.Fatalf("preview unexpectedly retains full dimensions: %v", decoded.Bounds())
		}
	}
	actual := comicRequest(t, service, file, page.URL+"?page=0")
	if actual.Code != http.StatusOK || !bytes.Equal(actual.Body.Bytes(), original) {
		t.Fatal("original comic page must retain its source bytes")
	}
	// Turning the feature off must not advertise or serve derivative URLs.
	service.cfg.Media.TranscodeCBZPages = false
	disabled := comicManifestForTest(t, comicRequest(t, service, file, "/api/v1/comics/book"))
	if disabled.Pages[0].Preview != "" || disabled.Pages[0].Lossless != "" {
		t.Fatalf("disabled derivatives still advertised: %+v", disabled.Pages[0])
	}
	if rec := comicRequest(t, service, file, page.Preview+"\u0026page=0"); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled preview status=%d, want 404", rec.Code)
	}
	if rec := comicRequest(t, service, file, page.URL+"?page=0"); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), original) {
		t.Fatal("disabled mode changed original page delivery")
	}
}

func TestComicPageLosslessVariantAndOriginalFallback(t *testing.T) {
	original := progressiveJPEGFixture(t)
	path := writeComic(t, map[string][]byte{"01.jpg": original})
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = t.TempDir()
	service := NewMediaService(cfg)
	service.losslessJPEGTool = writeJPEGTranscodeStub(t, "cat")
	file := types.FileInfo{Path: path, Hash: "comic-progressive"}
	page := comicManifestForTest(t, comicRequest(t, service, file, "/api/v1/comics/book")).Pages[0]
	if page.Lossless != page.URL+"?variant=lossless" || page.Preview == "" {
		t.Fatalf("progressive JPEG lacks expected variants: %+v", page)
	}
	for iteration, status := range []string{"miss", "hit"} {
		rec := comicRequest(t, service, file, page.Lossless+"\u0026page=0")
		if rec.Code != http.StatusOK || rec.Header().Get("X-Gooru-Cache") != status || rec.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("lossless request %d: status=%d cache=%q type=%q", iteration, rec.Code, rec.Header().Get("X-Gooru-Cache"), rec.Header().Get("Content-Type"))
		}
		if !bytes.Equal(rec.Body.Bytes(), original) {
			t.Fatal("test converter must preserve original page bytes")
		}
	}
	if rec := comicRequest(t, service, file, page.URL+"?page=0"); rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), original) {
		t.Fatal("lossless display route changed original page bytes")
	}
	service.cfg.Media.LosslessJPEGTranscode = false
	if got := comicManifestForTest(t, comicRequest(t, service, file, "/api/v1/comics/book")).Pages[0].Lossless; got != "" {
		t.Fatalf("disabled lossless derivative advertised: %q", got)
	}
	if rec := comicRequest(t, service, file, page.Lossless+"\u0026page=0"); rec.Code != http.StatusNotFound {
		t.Fatalf("disabled lossless status=%d", rec.Code)
	}
}

func TestComicPageCacheInvalidatesWhenActualArchiveVersionChanges(t *testing.T) {
	original := tinyPNG(t, 48, 24, color.White)
	path := writeComic(t, map[string][]byte{"01.png": original})
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Media.CacheDir = t.TempDir()
	cfg.Media.ThumbnailFormat = "png"
	cfg.Media.PreviewSize = 12
	service := NewMediaService(cfg)
	file := types.FileInfo{Path: path, Hash: "stale-tracked-hash"}
	target := "/api/v1/comics/book/0?variant=preview\u0026page=0"
	if got := comicRequest(t, service, file, target).Header().Get("X-Gooru-Cache"); got != "miss" {
		t.Fatalf("initial cache status=%q", got)
	}
	if got := comicRequest(t, service, file, target).Header().Get("X-Gooru-Cache"); got != "hit" {
		t.Fatalf("second cache status=%q", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := info.ModTime().Add(3 * time.Second)
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
	if got := comicRequest(t, service, file, target).Header().Get("X-Gooru-Cache"); got != "miss" {
		t.Fatalf("actual archive version change reused stale page derivative: cache=%q", got)
	}
}

func TestProtectedComicPageDerivativesUseEncryptedCache(t *testing.T) {
	original := progressiveJPEGFixture(t)
	path := writeComic(t, map[string][]byte{"01.jpg": original})
	file := types.FileInfo{ID: 943, Path: path, Hash: "protected-comic-variant"}
	server := protectedMediaTestServer(t, file)
	encryptMediaFixture(t, path, server.cfg.Encryption.Key)
	server.media.losslessJPEGTool = writeJPEGTranscodeStub(t, "cat")
	id := fallbackPublicFileID(file.ID)
	base := "/api/v1/comics/" + id
	manifestResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(manifestResponse, authedRequest(http.MethodGet, base))
	manifest := comicManifestForTest(t, manifestResponse)
	page := manifest.Pages[0]
	if page.Lossless == "" || page.Preview == "" {
		t.Fatalf("protected comic variants were not advertised: %+v", page)
	}
	for _, target := range []string{page.Preview, page.Lossless} {
		for iteration, status := range []string{"miss", "hit"} {
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, target))
			if rec.Code != http.StatusOK || rec.Header().Get("X-Gooru-Cache") != status {
				t.Fatalf("%s request %d status=%d cache=%q body=%s", target, iteration, rec.Code, rec.Header().Get("X-Gooru-Cache"), rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("protected derivative may be browser cached: %q", rec.Header().Get("Cache-Control"))
			}
			if bytes.Equal(rec.Body.Bytes(), original) && target == page.Preview {
				t.Fatal("protected preview incorrectly served the original image bytes")
			}
		}
	}
	archive, release, err := server.media.acquireComicArchive(file)
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"preview", "lossless"} {
		size, format := 0, "jpeg"
		if variant == "preview" {
			size = server.cfg.Media.PreviewSize
			format = server.cfg.Media.ThumbnailFormat
		}
		relative := server.media.comicPageDerivativePath(file, archive, 0, variant, size, format)
		stored, err := os.ReadFile(filepath.Join(server.cfg.Media.CacheDir, encryptedDerivativeNamespace, relative))
		if err != nil {
			release()
			t.Fatal(err)
		}
		if bytes.Contains(stored, original[:32]) {
			release()
			t.Fatalf("%s cached plaintext from protected archive", variant)
		}
		if len(stored) == 0 {
			release()
			t.Fatalf("empty protected %s cache", variant)
		}
	}
	release()
	if stored, err := os.ReadFile(path); err != nil || bytes.Equal(stored, original) {
		t.Fatalf("protected archive source was exposed: %v", err)
	}
}

// Keep a bound on test fixture clock changes across platforms with coarse mtimes.
var _ = time.Second
