package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"gooru.local/types"
)

func TestWebMUsesVideoMediaType(t *testing.T) {
	if got := mediaTypeForPath("clip.webm"); got != "video/webm" {
		t.Fatalf("expected video/webm, got %q", got)
	}
	if got := mediaKindForType(mediaTypeForPath("clip.webm")); got != "video" {
		t.Fatalf("expected video media kind, got %q", got)
	}
	if got := mediaTypeForPath("audio.weba"); got != "audio/webm" {
		t.Fatalf("expected audio/webm for .weba, got %q", got)
	}
}

func TestGIFPreviewServesAnimatedOriginal(t *testing.T) {
	body := []byte("GIF89a animated bytes")
	path := writeNamedMediaFile(t, "animated.gif", body)
	server := newMediaTestServer(t, types.FileInfo{ID: 41, Path: path, Hash: "gif-preview", Size: int64(len(body))})

	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(41)+"/preview")
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected GIF preview 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/gif" {
		t.Fatalf("expected image/gif, got %q", got)
	}
	if got := rec.Body.Bytes(); string(got) != string(body) {
		t.Fatalf("expected original GIF bytes, got %q", got)
	}
	if got := rec.Header().Get("X-Gooru-Cache"); got != "" {
		t.Fatalf("animated preview should bypass static derivative cache, got %q", got)
	}
}
