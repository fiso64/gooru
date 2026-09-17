package serve

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type durableUploadTestStore struct {
	producerClaimed     bool
	createErr           error
	createdRequest      core.BackgroundOperationRequest
	operation           core.BackgroundOperation
	state               core.BackgroundOperationState
	result              UploadImportResponse
	resultFound         bool
	attachCalls         int
	attachedRequest     core.BackgroundTaskRequest
	attachedCheckpoint  backgroundUploadCheckpoint
	attached            chan struct{}
	terminalOnAttach    core.BackgroundWorkStatus
	attachErr           error
	cancelCalls         int
	visibleCalls        int
	checkpointCalls     int
	receivingCheckpoint backgroundUploadCheckpoint
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

func (s *durableUploadTestStore) CreateBackgroundOperation(request core.BackgroundOperationRequest) (core.BackgroundOperation, error) {
	s.createdRequest = request
	if s.createErr != nil {
		return core.BackgroundOperation{}, s.createErr
	}
	return s.operation, nil
}

func (s *durableUploadTestStore) ClaimBackgroundOperationProducer(operationID string) (bool, error) {
	if operationID != s.operation.ID || s.state.Status != core.BackgroundWorkPending {
		return false, nil
	}
	if s.producerClaimed {
		return false, nil
	}
	s.producerClaimed = true
	return true, nil
}

func (s *durableUploadTestStore) SetBackgroundOperationCheckpoint(operationID string, checkpoint any) error {
	if operationID != s.operation.ID {
		return errors.New("unexpected operation")
	}
	s.checkpointCalls++
	s.receivingCheckpoint = checkpoint.(backgroundUploadCheckpoint)
	return nil
}

func (s *durableUploadTestStore) SetBackgroundOperationVisible(operationID string, visible bool) error {
	if operationID != s.operation.ID {
		return errors.New("unexpected operation")
	}
	s.visibleCalls++
	s.state.Visible = visible
	return nil
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
	if s.attachErr != nil {
		return core.BackgroundTask{}, s.attachErr
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

type gatedUploadBody struct {
	io.ReadCloser
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *gatedUploadBody) Read(p []byte) (int, error) {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})
	return b.ReadCloser.Read(p)
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

func singleDurableStagedPath(t *testing.T, stagingDir string) string {
	t.Helper()
	files := make([]string, 0, 1)
	err := filepath.WalkDir(stagingDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk durable staging directory: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("durable staging files = %d, want 1", len(files))
	}
	return files[0]
}

func TestClaimDurableUploadOperationRejectsDuplicateProducerClaim(t *testing.T) {
	store := newDurableUploadTestStore()
	store.state.Visible = true
	request := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	request.Header.Set(uploadOperationHeader, store.operation.ID)
	server := &Server{}

	if _, created, err := server.claimDurableUploadOperation(request, store); err != nil || created {
		t.Fatalf("first reservation claim: created=%v err=%v", created, err)
	}
	if _, _, err := server.claimDurableUploadOperation(request, store); err == nil {
		t.Fatal("duplicate reservation claim unexpectedly succeeded")
	}
	if store.cancelCalls != 0 {
		t.Fatalf("duplicate claim canceled operation %d times", store.cancelCalls)
	}
}

func TestDurableUploadOperationCreationFailureBeforeReadingOrStagingMultipart(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "uploads")
	store := newDurableUploadTestStore()
	store.createErr = errors.New("create failed")
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	body := &uploadReadTracker{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=unused")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusInternalServerError, "internal_error")
	if body.read {
		t.Fatal("request body was read after durable operation creation failed")
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("operation creation failure should not create upload staging directory: %v", err)
	}
	if store.attachCalls != 0 {
		t.Fatalf("attach calls = %d, want 0", store.attachCalls)
	}
}

func TestDurableUploadPublishesOperationWhileRequestBodyIsStillReceiving(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
	req.Header.Set("Prefer", "respond-async")
	body := &gatedUploadBody{ReadCloser: req.Body, entered: make(chan struct{}), release: make(chan struct{})}
	req.Body = body
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	<-body.entered
	if !store.state.Visible || store.visibleCalls != 1 {
		t.Fatalf("operation was not visible before body read completed: state=%+v calls=%d", store.state, store.visibleCalls)
	}
	if store.receivingCheckpoint.Phase != backgroundUploadPhaseReceiving {
		t.Fatalf("checkpoint phase while body blocked = %q, want receiving", store.receivingCheckpoint.Phase)
	}
	close(body.release)
	<-done
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
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
		t.Fatalf("unexpected operation request: %+v", store.createdRequest)
	}
	if store.visibleCalls != 1 || store.checkpointCalls < 1 || store.receivingCheckpoint.Phase != backgroundUploadPhaseReceiving {
		t.Fatalf("receiving publication = visible %d checkpoints %d checkpoint %+v", store.visibleCalls, store.checkpointCalls, store.receivingCheckpoint)
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
	stagingDir, err := durableUploadStagingDir(targetDir, store.operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	stagedPath := singleDurableStagedPath(t, stagingDir)
	if got := string(mustReadFile(t, stagedPath)); got != "hello" {
		t.Fatalf("clear-mode durable staging content = %q, want hello", got)
	}
}

func TestDurableUploadAsyncCancelReplaysCleanupAfterRestart(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}

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
		_ = client.Close()
		t.Fatalf("upload status = %d: %s", uploadRec.Code, uploadRec.Body.String())
	}
	var operation BackgroundOperationDTO
	if err := json.NewDecoder(uploadRec.Body).Decode(&operation); err != nil {
		_ = client.Close()
		t.Fatalf("decode upload operation: %v", err)
	}
	if operation.ID == "" || operation.Status != core.BackgroundWorkPending {
		_ = client.Close()
		t.Fatalf("unexpected pending upload operation: %+v", operation)
	}
	stagingDir, stagingErr := durableUploadStagingDir(uploadDir, operation.ID)
	if stagingErr != nil {
		_ = client.Close()
		t.Fatal(stagingErr)
	}
	stagedPath := singleDurableStagedPath(t, stagingDir)
	if got := string(mustReadFile(t, stagedPath)); got != "hello" {
		_ = client.Close()
		t.Fatalf("staged content = %q, want hello", got)
	}

	cancelRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(cancelRec, httptest.NewRequest(http.MethodDelete, "/api/v1/operations/"+operation.ID, nil))
	if cancelRec.Code != http.StatusAccepted {
		_ = client.Close()
		t.Fatalf("cancel status = %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	var canceled BackgroundOperationDTO
	if err := json.NewDecoder(cancelRec.Body).Decode(&canceled); err != nil {
		_ = client.Close()
		t.Fatalf("decode canceled operation: %v", err)
	}
	if canceled.ID != operation.ID || canceled.Status != core.BackgroundWorkCanceled {
		_ = client.Close()
		t.Fatalf("unexpected canceled operation: %+v", canceled)
	}
	if _, err := os.Stat(stagedPath); err != nil {
		_ = client.Close()
		t.Fatalf("staging should remain until durable cleanup runs: %v", err)
	}
	task, found, err := client.GetBackgroundOperationTask(operation.ID)
	if err != nil || !found {
		_ = client.Close()
		t.Fatalf("load canceled upload task = found %v err %v", found, err)
	}
	if task.Status != core.BackgroundWorkCanceled {
		_ = client.Close()
		t.Fatalf("canceled upload task status = %q, want %q", task.Status, core.BackgroundWorkCanceled)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close client before restart: %v", err)
	}

	restartedClient, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("reopen client: %v", err)
	}
	defer restartedClient.Close()
	restartedServer := NewServerWithLibrary(cfg, NewGooruLibrary(restartedClient, false))
	stopRuntime := startTestBackgroundRuntime(t, restartedServer, restartedClient, "upload-cancel-restart")
	defer stopRuntime()

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, statErr := os.Stat(stagingDir)
		if os.IsNotExist(statErr) {
			break
		}
		if statErr != nil {
			t.Fatalf("stat durable staging directory after restart: %v", statErr)
		}
		if time.Now().After(deadline) {
			t.Fatal("durable cancellation cleanup did not remove operation staging after restart")
		}
		time.Sleep(20 * time.Millisecond)
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
	attached := store.attached
	go func() {
		server.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	<-attached
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

func TestDurableUploadCancellationWinningAttachmentRaceReturnsCanceled(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	store.terminalOnAttach = core.BackgroundWorkCanceled
	store.attachErr = errors.New("operation canceled before attach")
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusRequestTimeout, "request_canceled")
	if _, err := os.Stat(filepath.Join(targetDir, "photo.jpg")); !os.IsNotExist(err) {
		t.Fatalf("staged file remained after canceled attachment race: %v", err)
	}
}
