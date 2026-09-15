package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestOriginalMediaMissingBackingFileReturnsNotFound(t *testing.T) {
	file := types.FileInfo{
		ID:   61,
		Path: filepath.Join(t.TempDir(), "missing.mp4"),
		Hash: "missing-source",
	}
	server := newMediaTestServer(t, file)

	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(file.ID)+"/content")
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing backing file to return 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOriginalMediaResolverFailureReturnsServiceUnavailable(t *testing.T) {
	path := writeNamedMediaFile(t, "video.mp4", []byte("media"))
	file := types.FileInfo{ID: 62, Path: path, Hash: "resolver-failure", Size: 5}
	server := newMediaTestServer(t, file)
	server.media.sourceResolverErr = errors.New("resolver initialization failed")

	for _, route := range []string{"content", "download"} {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := authedRequest(http.MethodGet, "/api/v1/files/"+fallbackPublicFileID(file.ID)+"/"+route)
			server.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected resolver failure to return 503, got %d: %s", rec.Code, rec.Body.String())
			}
			if body := rec.Body.String(); body == "" {
				t.Fatal("expected structured service-unavailable response")
			}
		})
	}
}
