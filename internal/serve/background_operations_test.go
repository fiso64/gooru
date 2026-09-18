package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "gooru.local/gooru"
)

type fakeBackgroundOperationReader struct {
	operations  []core.BackgroundOperationState
	byID        map[string]core.BackgroundOperationState
	results     map[string]json.RawMessage
	checkpoints map[string]backgroundUploadCheckpoint
	listOptions core.BackgroundOperationListOptions
	listErr     error
	getErr      error
	batchIDs    []string
	resultErr   error
	cancelErr   error
	canceledID  string
	cancelOK    bool
}

func (f *fakeBackgroundOperationReader) GetBackgroundOperation(id string) (core.BackgroundOperationState, bool, error) {
	if f.getErr != nil {
		return core.BackgroundOperationState{}, false, f.getErr
	}
	operation, ok := f.byID[id]
	return operation, ok, nil
}

func (f *fakeBackgroundOperationReader) GetBackgroundOperations(ids []string) (map[string]core.BackgroundOperationState, error) {
	f.batchIDs = append([]string(nil), ids...)
	return f.byID, nil
}

func (f *fakeBackgroundOperationReader) ListBackgroundOperations(options core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error) {
	f.listOptions = options
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.operations, nil
}

func (f *fakeBackgroundOperationReader) CancelBackgroundOperation(id string) (bool, error) {
	f.canceledID = id
	if f.cancelErr != nil {
		return false, f.cancelErr
	}
	if f.cancelOK {
		operation := f.byID[id]
		operation.Status = core.BackgroundWorkCanceled
		finishedAt := time.Now().UTC()
		operation.FinishedAt = &finishedAt
		f.byID[id] = operation
	}
	return f.cancelOK, nil
}

func (f *fakeBackgroundOperationReader) GetBackgroundOperationCheckpoint(id string, destination any) (bool, error) {
	checkpoint, ok := f.checkpoints[id]
	if !ok {
		return false, nil
	}
	*(destination.(*backgroundUploadCheckpoint)) = checkpoint
	return true, nil
}

func (f *fakeBackgroundOperationReader) GetBackgroundOperationResult(id string, destination any) (bool, error) {
	if f.resultErr != nil {
		return false, f.resultErr
	}
	result, ok := f.results[id]
	if !ok {
		return false, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, err
	}
	return true, nil
}

func TestHandleOperationsListsVisibleOperationsWithBoundedLimit(t *testing.T) {
	createdAt := time.Date(2026, 9, 8, 20, 0, 0, 0, time.UTC)
	reader := &fakeBackgroundOperationReader{operations: []core.BackgroundOperationState{
		{ID: "operation-visible", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkRunning, ProgressTotal: 3, ProgressCompleted: 1, CreatedAt: createdAt},
		{ID: "operation-hidden", Kind: "thumbnail", Visible: false, Status: core.BackgroundWorkPending, CreatedAt: createdAt},
	}}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=25", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !reader.listOptions.VisibleOnly || reader.listOptions.Limit != 25 {
		t.Fatalf("list options = %+v", reader.listOptions)
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "operation-visible" {
		t.Fatalf("items = %+v", payload.Items)
	}
	if payload.Items[0].ProgressTotal != 3 || payload.Items[0].ProgressCompleted != 1 {
		t.Fatalf("progress = %+v", payload.Items[0])
	}
}

func TestHandleOperationsRejectsInvalidLimit(t *testing.T) {
	server := &Server{backgroundOperations: &fakeBackgroundOperationReader{}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=1001", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleOperationHidesInvisibleOperation(t *testing.T) {
	reader := &fakeBackgroundOperationReader{byID: map[string]core.BackgroundOperationState{
		"operation-hidden": {ID: "operation-hidden", Kind: "thumbnail", Visible: false, Status: core.BackgroundWorkRunning, CreatedAt: time.Now().UTC()},
	}}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/operation-hidden", nil)
	response := httptest.NewRecorder()

	server.handleOperation(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleOperationReturnsVisibleState(t *testing.T) {
	finishedAt := time.Date(2026, 9, 8, 20, 5, 0, 0, time.UTC)
	reader := &fakeBackgroundOperationReader{byID: map[string]core.BackgroundOperationState{
		"operation-delete": {
			ID: "operation-delete", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkFailed,
			ProgressTotal: 2, ProgressCompleted: 1, ProgressFailed: 1,
			CreatedAt: finishedAt.Add(-time.Minute), FinishedAt: &finishedAt,
			ErrorCode: "partial_failure", ErrorMessage: "1 file failed",
		},
	}}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/operation-delete", nil)
	response := httptest.NewRecorder()

	server.handleOperation(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationDTO
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != core.BackgroundWorkFailed || payload.ProgressFailed != 1 || payload.ErrorCode != "partial_failure" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestHandleOperationReturnsCompletedStructuredResult(t *testing.T) {
	finishedAt := time.Now().UTC()
	reader := &fakeBackgroundOperationReader{
		byID: map[string]core.BackgroundOperationState{
			"operation-upload": {ID: "operation-upload", Kind: "upload_import", Visible: true, Status: core.BackgroundWorkCompleted, CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt},
		},
		results: map[string]json.RawMessage{
			"operation-upload": json.RawMessage(`{"affected_count":1,"files":[{"name":"a.jpg","status":"imported"}]}`),
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/operation-upload", nil)
	response := httptest.NewRecorder()

	server.handleOperation(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Result UploadImportResponse `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Result.AffectedCount != 1 || len(payload.Result.Files) != 1 || payload.Result.Files[0].Name != "a.jpg" {
		t.Fatalf("result = %+v", payload.Result)
	}
}

func TestHandleOperationCancelsVisibleActiveOperation(t *testing.T) {
	reader := &fakeBackgroundOperationReader{
		byID: map[string]core.BackgroundOperationState{
			"operation-delete": {ID: "operation-delete", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkRunning, CreatedAt: time.Now().UTC()},
		},
		cancelOK: true,
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/operation-delete", nil)
	response := httptest.NewRecorder()

	server.handleOperation(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if reader.canceledID != "operation-delete" {
		t.Fatalf("canceled id = %q", reader.canceledID)
	}
	var payload BackgroundOperationDTO
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != core.BackgroundWorkCanceled {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestHandleOperationRejectsCancelForInactiveOperation(t *testing.T) {
	reader := &fakeBackgroundOperationReader{byID: map[string]core.BackgroundOperationState{
		"operation-delete": {ID: "operation-delete", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkCompleted, CreatedAt: time.Now().UTC()},
	}}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/operation-delete", nil)
	response := httptest.NewRecorder()

	server.handleOperation(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleOperationDoesNotCancelHiddenOperation(t *testing.T) {
	reader := &fakeBackgroundOperationReader{byID: map[string]core.BackgroundOperationState{
		"operation-hidden": {ID: "operation-hidden", Kind: "thumbnail", Visible: false, Status: core.BackgroundWorkRunning, CreatedAt: time.Now().UTC()},
	}, cancelOK: true}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/operation-hidden", nil)
	response := httptest.NewRecorder()

	server.handleOperation(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if reader.canceledID != "" {
		t.Fatalf("hidden operation was canceled: %q", reader.canceledID)
	}
}

func TestHandleOperationsReportsReaderFailure(t *testing.T) {
	server := &Server{backgroundOperations: &fakeBackgroundOperationReader{listErr: errors.New("boom")}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleOperationsBatchesRequestedIDsWithCompletedResults(t *testing.T) {
	finishedAt := time.Now().UTC()
	reader := &fakeBackgroundOperationReader{
		getErr: errors.New("single operation read should not be used"),
		byID: map[string]core.BackgroundOperationState{
			"operation-upload":  {ID: "operation-upload", Kind: "upload_import", Visible: true, Status: core.BackgroundWorkCompleted, CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt},
			"operation-running": {ID: "operation-running", Kind: "upload_import", Visible: true, Status: core.BackgroundWorkRunning, CreatedAt: finishedAt.Add(-time.Second)},
			"operation-hidden":  {ID: "operation-hidden", Kind: "thumbnail", Visible: false, Status: core.BackgroundWorkCompleted, CreatedAt: finishedAt},
		},
		results: map[string]json.RawMessage{
			"operation-upload": json.RawMessage(`{"affected_count":1,"files":[{"name":"a.jpg","size":1,"target_id":"default","status":"imported"}]}`),
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?id=operation-running&id=operation-upload&id=operation-hidden&id=missing", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if len(reader.batchIDs) != 4 ||
		reader.batchIDs[0] != "operation-running" ||
		reader.batchIDs[1] != "operation-upload" ||
		reader.batchIDs[2] != "operation-hidden" ||
		reader.batchIDs[3] != "missing" {
		t.Fatalf("batch ids = %v", reader.batchIDs)
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 2 || payload.Items[0].ID != "operation-running" || payload.Items[1].ID != "operation-upload" {
		t.Fatalf("items = %+v", payload.Items)
	}
	var result UploadImportResponse
	if err := json.Unmarshal(payload.Items[1].Result, &result); err != nil {
		t.Fatalf("decode upload result: %v", err)
	}
	if result.AffectedCount != 1 || len(result.Files) != 1 || result.Files[0].Name != "a.jpg" {
		t.Fatalf("result = %+v", result)
	}
}

func TestHandleOperationsRejectsOversizedIDBatch(t *testing.T) {
	server := &Server{backgroundOperations: &fakeBackgroundOperationReader{}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	query := request.URL.Query()
	for i := 0; i <= maxBackgroundOperationStatusBatch; i++ {
		query.Add("id", fmt.Sprintf("operation-%d", i))
	}
	request.URL.RawQuery = query.Encode()
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleOperationsProjectsUploadFileProgressFromCheckpoint(t *testing.T) {
	createdAt := time.Now().UTC()
	reader := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{{
			ID: "operation-upload-progress", Kind: backgroundUploadImportOperationKind, Visible: true,
			Status: core.BackgroundWorkRunning, ProgressTotal: 1, CreatedAt: createdAt,
		}},
		checkpoints: map[string]backgroundUploadCheckpoint{
			"operation-upload-progress": {Phase: backgroundUploadPhaseActivated, FileTotal: 1000, FilesCompleted: 420},
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ProgressTotal != 1000 || payload.Items[0].ProgressCompleted != 420 {
		t.Fatalf("projected upload progress = %+v", payload.Items)
	}
}
