package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gooru.local/types"
)

func TestUploadRejectsDisabledUploads(t *testing.T) {
	server := newUploadTestServer(t, t.TempDir(), false, &recordingUploadLibrary{})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"a.txt": "hello"}, nil))

	assertAPIError(t, rec, http.StatusForbidden, "uploads_disabled")
}

func TestUploadRejectsDisabledUploadsBeforeParsingBody(t *testing.T) {
	server := newUploadTestServer(t, t.TempDir(), false, &recordingUploadLibrary{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", bytes.NewBufferString("not multipart"))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusForbidden, "uploads_disabled")
}

func TestUploadPreventsTraversalAndHandlesConflicts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "evil.txt"), []byte("existing"), 0600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, dir, true, library)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"../evil.txt": "uploaded"}, []string{"reviewed"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "evil-1.txt")); err != nil {
		t.Fatalf("expected conflict-safe upload name: %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(dir, "evil.txt"))); got != "existing" {
		t.Fatalf("existing file was overwritten: %q", got)
	}
	if len(library.paths) != 1 || filepath.Base(library.paths[0]) != "evil-1.txt" {
		t.Fatalf("unexpected imported paths: %+v", library.paths)
	}
	if len(library.tags) != 1 || library.tags[0] != "reviewed" {
		t.Fatalf("unexpected initial tags: %+v", library.tags)
	}
}

func TestUploadRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	server.cfg.Uploads.MaxFileSizeBytes = 3
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"a.txt": "hello"}, nil))

	assertAPIError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large")
	var payload ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	details, ok := payload.Error.Details.(map[string]interface{})
	if !ok || details["file"] != "a.txt" {
		t.Fatalf("expected per-file upload error details, got %#v", payload.Error.Details)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("oversized upload should be removed, entries=%v err=%v", entries, err)
	}
}

func TestUploadCleansEarlierFilesWhenBatchFails(t *testing.T) {
	dir := t.TempDir()
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	server.cfg.Uploads.MaxFileSizeBytes = 3
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{
		"a.txt": "ok",
		"b.txt": "too-large",
	}, nil))

	assertAPIError(t, rec, http.StatusRequestEntityTooLarge, "payload_too_large")
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("failed batch should not leave uploaded files, entries=%v err=%v", entries, err)
	}
}

func TestUploadAsyncReturnsJob(t *testing.T) {
	server := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"a.txt": "hello"}, nil)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var job Job
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if job.ID == "" || job.Type != "upload_import" {
		t.Fatalf("unexpected upload job: %+v", job)
	}
}

func TestUploadQueueFullReturnsStableJSONError(t *testing.T) {
	dir := t.TempDir()
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	release := saturateJobQueue(t, server)
	defer release()
	req := uploadRequest(t, map[string]string{"a.txt": "hello"}, nil)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusServiceUnavailable, "job_queue_full")
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("queue-full upload should clean staged files, entries=%v err=%v", entries, err)
	}
}

func TestUploadCleansStagedFilesWhenSubmissionContextIsCanceled(t *testing.T) {
	dir := t.TempDir()
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"a.txt": "hello"}, nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusRequestTimeout, "request_canceled")
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("canceled submission should clean staged files, entries=%v err=%v", entries, err)
	}
}

func TestCanceledQueuedUploadCleansStagedFiles(t *testing.T) {
	dir := t.TempDir()
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	server.jobs = NewJobManagerWithLimits(2, 1, 0, time.Hour)
	started := make(chan struct{})
	release := make(chan struct{})
	blocker, err := server.jobs.Submit(context.Background(), "blocker", true, func(ctx context.Context) (interface{}, error) {
		close(started)
		<-release
		return nil, nil
	})
	if err != nil {
		t.Fatalf("submit blocker: %v", err)
	}
	<-started

	req := uploadRequest(t, map[string]string{"a.txt": "hello"}, nil)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var uploadJob Job
	if err := json.Unmarshal(rec.Body.Bytes(), &uploadJob); err != nil {
		t.Fatalf("decode upload job: %v", err)
	}
	if _, ok := server.jobs.Cancel(uploadJob.ID); !ok {
		t.Fatal("cancel upload job")
	}
	close(release)
	waitForStatus(t, server.jobs, blocker.ID, JobCompleted)
	waitForStatus(t, server.jobs, uploadJob.ID, JobCanceled)

	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("canceled queued upload should clean staged files, entries=%v err=%v", entries, err)
	}
}

func newUploadTestServer(t *testing.T, dir string, enabled bool, library Library) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Token = "secret"
	cfg.Uploads.Enabled = enabled
	cfg.Uploads.Directories = []UploadDirectory{{Name: "default", Path: dir}}
	return NewServerWithLibrary(cfg, library)
}

func uploadRequest(t *testing.T, files map[string]string, tags []string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, content := range files {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	for _, tag := range tags {
		if err := writer.WriteField("tags", tag); err != nil {
			t.Fatalf("write tags field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return data
}

type recordingUploadLibrary struct {
	paths []string
	tags  []string
}

func (l *recordingUploadLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return nil, nil
}

func (l *recordingUploadLibrary) GetFile(_ context.Context, _ int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (l *recordingUploadLibrary) ListTags(_ context.Context, _ bool) ([]TagDTO, error) {
	return nil, nil
}

func (l *recordingUploadLibrary) ImportUploadedFiles(_ context.Context, paths []string, tags []string) (UploadImportResponse, error) {
	l.paths = paths
	l.tags = tags
	return UploadImportResponse{AffectedCount: len(paths)}, nil
}

var _ UploadLibrary = (*recordingUploadLibrary)(nil)
