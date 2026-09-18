package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
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

func TestTagMutationAsyncReturnsDurableOperation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()
	page := listTestFiles(t, server, "kind:image", 1)

	req := authedJSONRequest(http.MethodPut, "/api/v1/files/tags", `{"file_ids":["`+page.Files[0].ID+`"],"tags":["ready"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var operation BackgroundOperationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &operation); err != nil {
		t.Fatalf("decode operation: %v", err)
	}
	if operation.ID == "" || operation.Kind != core.BackgroundTagMutationOperationKind || operation.Status != core.BackgroundWorkPending {
		t.Fatalf("unexpected async operation: %+v", operation)
	}
	if got, want := rec.Header().Get("Location"), "/api/v1/operations/"+operation.ID; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestWaitForDurableTagMutationRequiresCompletedOperation(t *testing.T) {
	dir := t.TempDir()
	server, client := newTestBrowseServerAt(t, dir, filepath.Join(dir, "gooru.db"))
	defer client.Close()
	selector := TagMutationSelector{Query: "kind:image"}
	operation, err := client.CreateBackgroundTagMutation(core.BackgroundTagMutationRequest{
		Mutation: "add", Selector: selector, Tags: []string{"reviewed"},
		Query: selector.Query, MaxPending: defaultDurableTagMutationPendingLimit,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("persist result: %v", err)
	}
	shortCtx, shortCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer shortCancel()
	if _, err := waitForDurableTagMutation(shortCtx, server.backgroundOperations, operation.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait returned before durable task completion: %v", err)
	}
	runtime, err := server.NewBackgroundRuntime(client, "tag-mutation-completion-test")
	if err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(runCtx) }()
	defer func() {
		cancelRun()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("stop runtime: %v", err)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	response, err := waitForDurableTagMutation(ctx, server.backgroundOperations, operation.ID)
	if err != nil {
		t.Fatalf("wait for completed mutation: %v", err)
	}
	if response.MatchedFiles < 1 || response.AffectedCount < 1 || response.Operation != TagOperationAdd {
		t.Fatalf("unexpected completed result: %+v", response)
	}
}
func TestTagMutationQueueFullReturnsStableJSONError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()
	page := listTestFiles(t, server, "kind:image", 1)

	for i := 0; i < defaultDurableTagMutationPendingLimit; i++ {
		if _, err := client.CreateBackgroundOperationWithPendingLimit(core.BackgroundOperationRequest{
			Kind:          core.BackgroundTagMutationOperationKind,
			Visible:       true,
			ProgressTotal: 1,
		}, defaultDurableTagMutationPendingLimit+1); err != nil {
			t.Fatalf("seed pending operation %d: %v", i, err)
		}
	}
	req := authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+page.Files[0].ID+`"],"tags":["reviewed"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusServiceUnavailable, "job_queue_full")
}

func TestTagMutationIntegrationUpdatesFileTags(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()
	runtime, err := server.NewBackgroundRuntime(client, "tag-mutation-test")
	if err != nil {
		t.Fatalf("create background runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("stop background runtime: %v", err)
		}
	}()

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

func (l *recordingMutationLibrary) ListTags(_ context.Context, _ bool, _ int) ([]TagDTO, error) {
	return nil, nil
}

func (l *recordingMutationLibrary) PublicFileID(file types.FileInfo) string {
	return fallbackPublicFileID(file.ID)
}

func (l *recordingMutationLibrary) GetFileByPublicID(ctx context.Context, id string) (types.FileInfo, error) {
	locationID, err := fallbackResolveFileID(id)
	if err != nil {
		return types.FileInfo{}, err
	}
	return l.GetFile(ctx, locationID)
}

func (l *recordingMutationLibrary) DeleteFileByPublicID(ctx context.Context, id string) (bool, error) {
	_, err := l.GetFileByPublicID(ctx, id)
	if err != nil {
		return false, err
	}
	return true, nil
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
