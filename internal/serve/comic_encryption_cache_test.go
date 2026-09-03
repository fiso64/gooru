package serve

import (
	"bytes"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"gooru.local/types"
)

func TestProtectedComicPageDisablesBrowserCaching(t *testing.T) {
	page := tinyPNG(t, 3, 2, color.RGBA{R: 255, A: 255})
	path := writeComic(t, map[string][]byte{"01.png": page})
	file := types.FileInfo{ID: 94, Path: path, Hash: "protected-comic"}
	server := protectedMediaTestServer(t, file)
	encryptMediaFixture(t, path, server.cfg.Encryption.Key)

	recorder := httptest.NewRecorder()
	request := authedRequest(http.MethodGet, "/api/v1/comics/"+fallbackPublicFileID(94)+"/0")
	server.Handler().ServeHTTP(recorder, request)

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
	if !bytes.Equal(recorder.Body.Bytes(), page) {
		t.Fatal("served decrypted page bytes differ from archive entry")
	}
}

func TestOrdinaryComicPageRetainsShortPrivateCache(t *testing.T) {
	page := tinyPNG(t, 2, 2, color.White)
	path := writeComic(t, map[string][]byte{"01.png": page})
	server := newMediaTestServer(t, types.FileInfo{ID: 95, Path: path, Hash: "ordinary-comic"})

	recorder := httptest.NewRecorder()
	request := authedRequest(http.MethodGet, "/api/v1/comics/"+fallbackPublicFileID(95)+"/0")
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Fatalf("Cache-Control = %q, want private, max-age=3600", got)
	}
}
