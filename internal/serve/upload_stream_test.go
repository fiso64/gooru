package serve

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadStreamsLargeMultipartWithoutOSTempSpill(t *testing.T) {
	uploadDir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, uploadDir, true, library)
	server.cfg.Server.MaxRequestBodyBytes = 64 << 20
	server.cfg.Uploads.MaxFileSizeBytes = 48 << 20

	badTemp := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badTemp, []byte("x"), 0600); err != nil {
		t.Fatalf("write temp sentinel: %v", err)
	}
	t.Setenv("TMPDIR", badTemp)

	payload := bytes.Repeat([]byte("x"), 40<<20)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadBinaryRequest(t, map[string][]byte{"large.bin": payload}, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected streamed upload to succeed without OS temp, got %d: %s", rec.Code, rec.Body.String())
	}
	info, err := os.Stat(filepath.Join(uploadDir, "large.bin"))
	if err != nil {
		t.Fatalf("stat uploaded file: %v", err)
	}
	if info.Size() != int64(len(payload)) {
		t.Fatalf("uploaded size=%d want=%d", info.Size(), len(payload))
	}
	if len(library.files) != 1 || library.files[0].Size != int64(len(payload)) {
		t.Fatalf("unexpected staged files: %+v", library.files)
	}
}

func TestUploadStreamingAcceptsMetadataAfterFilePart(t *testing.T) {
	defaultDir := t.TempDir()
	archiveDir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, defaultDir, true, library)
	server.cfg.Uploads.Targets = append(server.cfg.Uploads.Targets, UploadTarget{ID: "archive", Name: "Archive", Path: archiveDir})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("target_id", "archive"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("conflict_policy", "rename"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(mustReadFile(t, filepath.Join(archiveDir, "a.txt"))); got != "hello" {
		t.Fatalf("archive upload=%q", got)
	}
	entries, err := os.ReadDir(defaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("managed staging should be cleaned, entries=%v", entries)
	}
}

func TestUploadReportsMalformedMultipartReadFailure(t *testing.T) {
	server := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	boundary := writer.Boundary()
	body.WriteString("--" + boundary + "\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.txt\"\r\nContent-Type: text/plain\r\n\r\npartial")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var response ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Error.Message, "unexpected EOF") {
		t.Fatalf("expected concrete multipart read diagnostic, got %q", response.Error.Message)
	}
}
