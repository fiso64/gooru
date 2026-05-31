package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestTagMutationRejectsAmbiguousSelector(t *testing.T) {
	server := newTagMutationTestServer(t, &recordingMutationLibrary{file: types.FileInfo{ID: 1, Path: "/tmp/a.jpg"}})
	body := `{"file_ids":["` + fallbackPublicFileID(1) + `"],"query":"kind:image","tags":["reviewed"]}`
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, authedJSONRequest(http.MethodPost, "/api/v1/files/tags", body))

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestTagMutationRejectsInvalidQuery(t *testing.T) {
	server := newTagMutationTestServer(t, &recordingMutationLibrary{})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"query":"(","tags":["reviewed"]}`))

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_query")
}

func TestTagMutationSyncByFileID(t *testing.T) {
	fileID := fallbackPublicFileID(5)
	library := &recordingMutationLibrary{file: types.FileInfo{ID: 5, Path: "/tmp/a.jpg"}}
	server := newTagMutationTestServer(t, library)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+fileID+`"],"tags":["reviewed"]}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response TagMutationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Operation != TagOperationAdd || response.AffectedCount != 2 || response.MatchedFiles != 1 {
		t.Fatalf("unexpected mutation response: %+v", response)
	}
	if library.operation != TagOperationAdd || len(library.request.FileIDs) != 1 || library.request.Tags[0] != "reviewed" {
		t.Fatalf("mutation was not forwarded correctly: operation=%q request=%+v", library.operation, library.request)
	}
}

func TestTagMutationAsyncReturnsJob(t *testing.T) {
	server := newTagMutationTestServer(t, &recordingMutationLibrary{file: types.FileInfo{ID: 6, Path: "/tmp/a.jpg"}})
	req := authedJSONRequest(http.MethodPut, "/api/v1/files/tags", `{"file_ids":["`+fallbackPublicFileID(6)+`"],"tags":["ready"]}`)
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
	if job.ID == "" || job.Type != "tag_mutation" {
		t.Fatalf("unexpected async job: %+v", job)
	}
}

func TestTagMutationQueueFullReturnsStableJSONError(t *testing.T) {
	server := newTagMutationTestServer(t, &recordingMutationLibrary{file: types.FileInfo{ID: 7, Path: "/tmp/a.jpg"}})
	release := saturateJobQueue(t, server)
	defer release()
	req := authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+fallbackPublicFileID(7)+`"],"tags":["reviewed"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusServiceUnavailable, "job_queue_full")
}

func TestTagMutationIntegrationUpdatesFileTags(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, authedRequest(http.MethodGet, "/api/v1/files?query=kind:image&limit=1"))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(page.Files))
	}

	mutateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(mutateRec, authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+page.Files[0].ID+`"],"tags":["reviewed"]}`))
	if mutateRec.Code != http.StatusOK {
		t.Fatalf("expected mutation 200, got %d: %s", mutateRec.Code, mutateRec.Body.String())
	}

	detailRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(detailRec, authedRequest(http.MethodGet, "/api/v1/files/"+page.Files[0].ID))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	var detail FileDTO
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if !containsString(detail.Tags, "reviewed") {
		t.Fatalf("expected updated tags to include reviewed, got %+v", detail.Tags)
	}
}

func newTagMutationTestServer(t *testing.T, library Library) *Server {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	return NewServerWithLibrary(cfg, library)
}

func authedJSONRequest(method string, target string, body string) *http.Request {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

type recordingMutationLibrary struct {
	file      types.FileInfo
	operation TagOperation
	request   TagMutationRequest
}

func (l *recordingMutationLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return nil, nil
}

func (l *recordingMutationLibrary) GetFile(_ context.Context, id int64) (types.FileInfo, error) {
	if id != l.file.ID {
		return types.FileInfo{}, ErrNotFound
	}
	return l.file, nil
}

func (l *recordingMutationLibrary) ListTags(_ context.Context, _ bool) ([]TagDTO, error) {
	return nil, nil
}

func (l *recordingMutationLibrary) PublicFileID(file types.FileInfo) string {
	return fallbackPublicFileID(file.ID)
}

func (l *recordingMutationLibrary) ResolveFileID(_ context.Context, id string) (int64, error) {
	return fallbackResolveFileID(id)
}

func (l *recordingMutationLibrary) MutateTags(_ context.Context, operation TagOperation, request TagMutationRequest) (TagMutationResponse, error) {
	l.operation = operation
	l.request = request
	return TagMutationResponse{
		Operation:     operation,
		Selector:      TagMutationSelector{FileIDs: request.FileIDs, Query: request.Query},
		MatchedFiles:  len(request.FileIDs),
		AffectedCount: 2,
	}, nil
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

var _ TagMutationLibrary = (*recordingMutationLibrary)(nil)
