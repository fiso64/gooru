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
	"strings"
	"testing"
	"time"

	core "gooru.local/gooru"
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
	if len(library.files) != 1 || library.files[0].TargetID != "default" {
		t.Fatalf("unexpected staged upload files: %+v", library.files)
	}
	if len(library.tags) != 1 || library.tags[0] != "reviewed" {
		t.Fatalf("unexpected initial tags: %+v", library.tags)
	}
}

func TestUploadUsesRequestedTargetID(t *testing.T) {
	defaultDir := t.TempDir()
	archiveDir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, defaultDir, true, library)
	server.cfg.Uploads.Targets = append(server.cfg.Uploads.Targets, UploadTarget{ID: "archive", Name: "Archive", Path: archiveDir})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithTarget(t, map[string]string{"a.txt": "hello"}, nil, "archive"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(archiveDir, "a.txt")); err != nil {
		t.Fatalf("expected upload in requested target: %v", err)
	}
	if _, err := os.Stat(filepath.Join(defaultDir, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("default target should not receive requested upload, err=%v", err)
	}
	if len(library.files) != 1 || library.files[0].TargetID != "archive" {
		t.Fatalf("unexpected target passed to importer: %+v", library.files)
	}
}

func TestUploadRejectsUnknownTargetID(t *testing.T) {
	server := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithTarget(t, map[string]string{"a.txt": "hello"}, nil, "missing"))

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_upload_target")
}

func TestUploadTargetsEndpointHidesPaths(t *testing.T) {
	server := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/upload-targets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload UploadTargetsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode targets: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "default" || payload.Items[0].Name != "Default" {
		t.Fatalf("unexpected targets response: %+v", payload)
	}
	if strings.Contains(rec.Body.String(), server.cfg.Uploads.Targets[0].Path) {
		t.Fatalf("targets response exposed absolute path: %s", rec.Body.String())
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

func TestGooruUploadImportReportsDuplicateStatuses(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()
	existing := writeTestFile(t, dir, "existing-source.txt", "already tracked")
	if _, err := client.TagFiles([]string{existing}, []string{"state:existing"}, nil, false); err != nil {
		t.Fatalf("tag existing file: %v", err)
	}
	uploadDir := filepath.Join(dir, "uploads")
	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{
		"duplicate-existing.txt": "already tracked",
		"batch-a.txt":            "new duplicated content",
		"batch-b.txt":            "new duplicated content",
	}, []string{"uploaded"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	statuses := make(map[string]string)
	for _, file := range response.Files {
		statuses[file.Name] = file.Status
		if file.TargetID != "default" {
			t.Fatalf("unexpected target for %s: %+v", file.Name, file)
		}
	}
	if statuses["duplicate-existing.txt"] != "duplicate_existing" {
		t.Fatalf("expected duplicate_existing, got statuses %+v", statuses)
	}
	batchStatuses := []string{statuses["batch-a.txt"], statuses["batch-b.txt"]}
	if !containsStringValue(batchStatuses, "imported") || !containsStringValue(batchStatuses, "duplicate_in_batch") {
		t.Fatalf("expected one imported and one duplicate_in_batch, got %+v", statuses)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, "duplicate-existing.txt")); !os.IsNotExist(err) {
		t.Fatalf("duplicate existing upload should be removed, err=%v", err)
	}
}

func newUploadTestServer(t *testing.T, dir string, enabled bool, library Library) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Uploads.Enabled = enabled
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: dir}}
	return NewServerWithLibrary(cfg, library)
}

func uploadRequest(t *testing.T, files map[string]string, tags []string) *http.Request {
	return uploadRequestWithTarget(t, files, tags, "")
}

func uploadRequestWithTarget(t *testing.T, files map[string]string, tags []string, targetID string) *http.Request {
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
	if targetID != "" {
		if err := writer.WriteField("target_id", targetID); err != nil {
			t.Fatalf("write target_id field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
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
	files []StagedUpload
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

func (l *recordingUploadLibrary) ImportUploadedFiles(_ context.Context, files []StagedUpload, tags []string) (UploadImportResponse, error) {
	l.files = files
	for _, file := range files {
		l.paths = append(l.paths, file.Path)
	}
	l.tags = tags
	response := UploadImportResponse{AffectedCount: len(files)}
	for _, file := range files {
		response.Files = append(response.Files, UploadedFileDTO{Name: file.Name, Size: file.Size, TargetID: file.TargetID, Status: "imported"})
	}
	return response, nil
}

var _ UploadLibrary = (*recordingUploadLibrary)(nil)

func containsStringValue(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
