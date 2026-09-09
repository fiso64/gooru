package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "gooru.local/gooru"
)

type fakeBackgroundOperationReader struct {
	operations  []core.BackgroundOperationState
	byID        map[string]core.BackgroundOperationState
	listOptions core.BackgroundOperationListOptions
	listErr     error
	getErr      error
}

func (f *fakeBackgroundOperationReader) GetBackgroundOperation(id string) (core.BackgroundOperationState, bool, error) {
	if f.getErr != nil {
		return core.BackgroundOperationState{}, false, f.getErr
	}
	operation, ok := f.byID[id]
	return operation, ok, nil
}

func (f *fakeBackgroundOperationReader) ListBackgroundOperations(options core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error) {
	f.listOptions = options
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.operations, nil
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

func TestHandleOperationsReportsReaderFailure(t *testing.T) {
	server := &Server{backgroundOperations: &fakeBackgroundOperationReader{listErr: errors.New("boom")}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}
