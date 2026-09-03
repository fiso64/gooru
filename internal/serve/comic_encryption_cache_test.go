package serve

import (
	"image/color"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func TestProtectedComicPageDisablesBrowserCaching(t *testing.T) {
	page := tinyPNG(t, 3, 2, color.RGBA{R: 255, A: 255})
	path := writeComic(t, map[string][]byte{"01.png": page})
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = make([]byte, securekey.Size)
	encryptMediaFixture(t, path, cfg.Encryption.Key)
	service := NewMediaService(cfg)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/comics/file-id/0?page=0", nil)
	service.ServeComic(recorder, request, types.FileInfo{Path: path, Hash: "protected-comic"}, "file-id")

	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", got)
	}
	if got := recorder.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	if got := recorder.Header().Get("Expires"); got != "0" {
		t.Fatalf("Expires = %q, want 0", got)
	}
	if string(recorder.Body.Bytes()) != string(page) {
		t.Fatal("served decrypted page bytes differ from archive entry")
	}
}

func TestOrdinaryComicPageRetainsShortPrivateCache(t *testing.T) {
	page := tinyPNG(t, 2, 2, color.White)
	path := writeComic(t, map[string][]byte{"01.png": page})
	service := NewMediaService(DefaultConfig(filepath.Join(t.TempDir(), "gooru.db")))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/comics/file-id/0?page=0", nil)
	service.ServeComic(recorder, request, types.FileInfo{Path: path, Hash: "ordinary-comic"}, "file-id")

	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want private, max-age=3600", got)
	}
}
