package serve

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"gooru.local/types"
)

func TestOriginalMediaRoutesSupportHead(t *testing.T) {
	body := []byte("0123456789")
	path := writeNamedMediaFile(t, "track.mp3", body)
	file := types.FileInfo{ID: 51, Path: path, Hash: "head-original", Size: int64(len(body))}
	server := newMediaTestServer(t, file)
	id := fallbackPublicFileID(file.ID)

	for _, tc := range []struct {
		name            string
		route           string
		wantDisposition bool
	}{
		{name: "content", route: "content"},
		{name: "download", route: "download", wantDisposition: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := authedRequest(http.MethodHead, "/api/v1/files/"+id+"/"+tc.route)

			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected HEAD 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("HEAD response included %d body bytes", rec.Body.Len())
			}
			if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
				t.Fatalf("Content-Length = %q, want %d", got, len(body))
			}
			if got := rec.Header().Get("Content-Type"); got != "audio/mpeg" {
				t.Fatalf("Content-Type = %q, want audio/mpeg", got)
			}
			disposition := rec.Header().Get("Content-Disposition")
			if tc.wantDisposition && !strings.Contains(disposition, "attachment") {
				t.Fatalf("download HEAD Content-Disposition = %q, want attachment", disposition)
			}
			if !tc.wantDisposition && disposition != "" {
				t.Fatalf("content HEAD Content-Disposition = %q, want empty", disposition)
			}
		})
	}
}

func TestNestedMediaRoutesAdvertiseSupportedMethods(t *testing.T) {
	body := []byte("0123456789")
	path := writeNamedMediaFile(t, "track.mp3", body)
	file := types.FileInfo{ID: 52, Path: path, Hash: "media-methods", Size: int64(len(body))}
	server := newMediaTestServer(t, file)
	id := fallbackPublicFileID(file.ID)

	for _, tc := range []struct {
		name   string
		method string
		route  string
		allow  string
	}{
		{name: "content", method: http.MethodPost, route: "content", allow: "GET, HEAD"},
		{name: "download", method: http.MethodDelete, route: "download", allow: "GET, HEAD"},
		{name: "thumbnail", method: http.MethodHead, route: "thumbnail", allow: "GET"},
		{name: "preview", method: http.MethodPost, route: "preview", allow: "GET"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := authedRequest(tc.method, "/api/v1/files/"+id+"/"+tc.route)

			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Allow"); got != tc.allow {
				t.Fatalf("Allow = %q, want %q", got, tc.allow)
			}
		})
	}
}
