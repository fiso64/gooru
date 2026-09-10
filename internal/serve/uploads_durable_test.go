package serve

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type durableUploadTestStore struct {
	createErr          error
	createdRequest     core.BackgroundOperationRequest
	createdLimit       int
	operation          core.BackgroundOperation
	state              core.BackgroundOperationState
	result             UploadImportResponse
	resultFound        bool
	attachCalls        int
	attachedRequest    core.BackgroundTaskRequest
	attachedCheckpoint backgroundUploadCheckpoint
	attached           chan struct{}
	terminalOnAttach   core.BackgroundWorkStatus
	cancelCalls        int
}

func newDurableUploadTestStore() *durableUploadTestStore {
	return &durableUploadTestStore{
		operation: core.BackgroundOperation{ID: "operation-upload-test", Kind: "upload_import", ProgressTotal: 1},
		state: core.BackgroundOperationState{
			ID:            "operation-upload-test",
			Kind:          "upload_import",
			Visible:       false,
			Status:        core.BackgroundWorkPending,
			ProgressTotal: 1,
		},
		attached: make(chan struct{}),
	}
}

func (s *durableUploadTestStore) CreateBackgroundOperationWithPendingLimit(request core.BackgroundOperationRequest, limit int) (core.BackgroundOperation, error) {
	s.createdRequest = request
	s.createdLimit = limit
	if s.createErr != nil {
		return core.BackgroundOperation{}, s.createErr
	}
	return s.operation, nil
}

func (s *durableUploadTestStore) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, error) {
	s.attachCalls++
	s.attachedRequest = request
	s.attachedCheckpoint = checkpoint.(backgroundUploadCheckpoint)
	s.state.Visible = true
	if s.terminalOnAttach != "" {
		s.state.Status = s.terminalOnAttach
	}
	if s.attached != nil {
		close(s.attached)
		s.attached = nil
	}
	return core.BackgroundTask{ID: "task-upload-test", OperationID: operationID, Kind: request.Kind, SubjectKind: request.SubjectKind, SubjectID: request.SubjectID, InputKey: request.InputKey}, nil
}

func (s *durableUploadTestStore) GetBackgroundOperation(operationID string) (core.BackgroundOperationState, bool, error) {
	if operationID != s.operation.ID {
		return core.BackgroundOperationState{}, false, nil
	}
	return s.state, true, nil
}

func (s *durableUploadTestStore) ListBackgroundOperations(core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error) {
	return []core.BackgroundOperationState{s.state}, nil
}

func (s *durableUploadTestStore) CancelBackgroundOperation(operationID string) (bool, error) {
	if operationID != s.operation.ID {
		return false, nil
	}
	s.cancelCalls++
	s.state.Status = core.BackgroundWorkCanceled
	return true, nil
}

func (s *durableUploadTestStore) GetBackgroundOperationResult(operationID string, destination any) (bool, error) {
	if operationID != s.operation.ID || !s.resultFound {
		return false, nil
	}
	result := destination.(*UploadImportResponse)
	*result = s.result
	return true, nil
}

type uploadReadTracker struct {
	read bool
}

func (r *uploadReadTracker) Read([]byte) (int, error) {
	r.read = true
	return 0, io.EOF
}

func (r *uploadReadTracker) Close() error { return nil }

func newDurableUploadHandlerTestServer(t *testing.T, targetDir string, store *durableUploadTestStore) *Server {
	t.Helper()
	server := newUploadTestServer(t, targetDir, true, &recordingUploadLibrary{})
	server.backgroundOperations = store
	return server
}

func TestDurableUploadRejectsAdmissionBeforeReadingOrStagingMultipart(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "uploads")
	store := newDurableUploadTestStore()
	store.createErr = core.ErrBackgroundOperationPendingLimit
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	body := &uploadReadTracker{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=unused")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusServiceUnavailable, "job_queue_full")
	if body.read {
		t.Fatal("request body was read after durable admission was rejected")
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("admission rejection should not create upload staging directory: %v", err)
	}
	if store.attachCalls != 0 {
		t.Fatalf("attach calls = %d, want 0", store.attachCalls)
	}
}

func TestDurableUploadAsyncAttachesOneTaskAndReturnsOperationLocation(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, []string{"source:upload"})
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got, want := rec.Header().Get("Location"), "/api/v1/operations/"+store.operation.ID; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
	var response BackgroundOperationDTO
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != store.operation.ID || response.Kind != "upload_import" || response.Status != core.BackgroundWorkPending {
		t.Fatalf("unexpected operation response: %+v", response)
	}
	if store.createdRequest.Kind != "upload_import" || store.createdRequest.Visible || store.createdRequest.ProgressTotal != 1 {
		t.Fatalf("unexpected admission request: %+v", store.createdRequest)
	}
	if store.createdLimit != server.durableUploadPendingLimit() {
		t.Fatalf("pending limit = %d, want %d", store.createdLimit, server.durableUploadPendingLimit())
	}
	if store.attachCalls != 1 {
		t.Fatalf("attach calls = %d, want 1", store.attachCalls)
	}
	if store.attachedCheckpoint.Phase != backgroundUploadPhaseStaged {
		t.Fatalf("checkpoint phase = %q, want %q", store.attachedCheckpoint.Phase, backgroundUploadPhaseStaged)
	}
	if store.attachedRequest.Kind != backgroundUploadTaskKind || store.attachedRequest.SubjectID != store.operation.ID || store.attachedRequest.ResourceClass != backgroundUploadResourceClass {
		t.Fatalf("unexpected durable task request: %+v", store.attachedRequest)
	}
	stagedPath := filepath.Join(targetDir, "photo.jpg")
	if got := string(mustReadFile(t, stagedPath)); got != "hello" {
		t.Fatalf("clear-mode durable staging content = %q, want hello", got)
	}
}

func TestDurableUploadAsyncCancelRemovesNeverClaimedStaging(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	uploadDir := filepath.Join(root, "uploads")
	cfg := DefaultConfig(dbPath)
	cfg.Auth.Enabled = false
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

	upload := uploadRequest(t, map[string]string{"pending.txt": "hello"}, nil)
	upload.Header.Set("Prefer", "respond-async")
	uploadRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(uploadRec, upload)
	if uploadRec.Code != http.StatusAccepted {
		t.Fatalf("upload status = %d: %s", uploadRec.Code, uploadRec.Body.String())
	}
	var operation BackgroundOperationDTO
	if err := json.NewDecoder(uploadRec.Body).Decode(&operation); err != nil {
		t.Fatalf("decode upload operation: %v", err)
	}
	if operation.ID == "" || operation.Status != core.BackgroundWorkPending {
		t.Fatalf("unexpected pending upload operation: %+v", operation)
	}
	stagedPath := filepath.Join(uploadDir, "pending.txt")
	if got := string(mustReadFile(t, stagedPath)); got != "hello" {
		t.Fatalf("staged content = %q, want hello", got)
	}

	cancelRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(cancelRec, httptest.NewRequest(http.MethodDelete, "/api/v1/operations/"+operation.ID, nil))
	if cancelRec.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	var canceled BackgroundOperationDTO
	if err := json.NewDecoder(cancelRec.Body).Decode(&canceled); err != nil {
		t.Fatalf("decode canceled operation: %v", err)
	}
	if canceled.ID != operation.ID || canceled.Status != core.BackgroundWorkCanceled {
		t.Fatalf("unexpected canceled operation: %+v", canceled)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("canceled pending upload left staged file behind: %v", err)
	}
	task, found, err := client.GetBackgroundOperationTask(operation.ID)
	if err != nil || !found {
		t.Fatalf("load canceled upload task = found %v err %v", found, err)
	}
	if task.Status != core.BackgroundWorkCanceled {
		t.Fatalf("canceled upload task status = %q, want %q", task.Status, core.BackgroundWorkCanceled)
	}
}

func TestDurableUploadSyncReturnsPersistedOperationResult(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	store.terminalOnAttach = core.BackgroundWorkCompleted
	store.resultFound = true
	store.result = UploadImportResponse{
		Files:         []UploadedFileDTO{{Name: "photo.jpg", Size: 5, TargetID: "default", Status: "imported"}},
		AffectedCount: 1,
	}
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(response, store.result) {
		t.Fatalf("response = %#v, want %#v", response, store.result)
	}
}

func TestDurableUploadSyncCancellationCancelsOperation(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	<-store.attached
	cancel()
	<-done

	assertAPIError(t, rec, http.StatusRequestTimeout, "request_canceled")
	if store.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", store.cancelCalls)
	}
}

func TestDurableUploadSyncFailureReturnsStableError(t *testing.T) {
	store := newDurableUploadTestStore()
	store.terminalOnAttach = core.BackgroundWorkFailed
	store.state.ErrorMessage = "import failed"
	server := newDurableUploadHandlerTestServer(t, t.TempDir(), store)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil))

	assertAPIError(t, rec, http.StatusInternalServerError, "internal_error")
}
