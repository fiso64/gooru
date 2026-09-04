package serve

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestUploadPreservesReportedSourceModTimeAndCarriesMetadata(t *testing.T) {
	dir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, dir, true, library)
	server.cfg.Uploads.PreserveModTime = true
	sourceModTime := time.Date(2001, time.February, 3, 4, 5, 6, 789000000, time.UTC)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadWithSourceModTimeRequest(t, "photo.jpg", []byte("payload"), sourceModTime))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	info, err := os.Stat(filepath.Join(dir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(sourceModTime) {
		t.Fatalf("destination modtime=%s want=%s", info.ModTime(), sourceModTime)
	}
	if len(library.files) != 1 || !library.files[0].SourceModTime.Equal(sourceModTime) {
		t.Fatalf("source modtime metadata was not carried to importer: %+v", library.files)
	}
}

func TestUploadCarriesSourceModTimeWithoutChangingDestinationWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, dir, true, library)
	server.cfg.Uploads.PreserveModTime = false
	sourceModTime := time.Date(2001, time.February, 3, 4, 5, 6, 789000000, time.UTC)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadWithSourceModTimeRequest(t, "photo.jpg", []byte("payload"), sourceModTime))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	info, err := os.Stat(filepath.Join(dir, "photo.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if info.ModTime().Equal(sourceModTime) {
		t.Fatalf("destination unexpectedly inherited source modtime while preserve_modtime=false")
	}
	if len(library.files) != 1 || !library.files[0].SourceModTime.Equal(sourceModTime) {
		t.Fatalf("source modtime metadata was not retained for later added_at policy: %+v", library.files)
	}
}

func uploadWithSourceModTimeRequest(t *testing.T, name string, data []byte, sourceModTime time.Time) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("source_mod_time_ms", strconv.FormatInt(sourceModTime.UnixMilli(), 10)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}
