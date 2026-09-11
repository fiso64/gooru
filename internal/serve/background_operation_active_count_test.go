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

type exactActiveCountReader struct {
	*fakeBackgroundOperationReader
	activeCount int
	countErr    error
	visibleOnly bool
}

func (r *exactActiveCountReader) CountActiveBackgroundOperations(visibleOnly bool) (int, error) {
	r.visibleOnly = visibleOnly
	if r.countErr != nil {
		return 0, r.countErr
	}
	return r.activeCount, nil
}

func TestHandleOperationsUsesExactActiveCountBeyondReturnedPrefix(t *testing.T) {
	reader := &exactActiveCountReader{
		fakeBackgroundOperationReader: &fakeBackgroundOperationReader{operations: []core.BackgroundOperationState{
			{ID: "newest-terminal", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkCompleted, CreatedAt: time.Now().UTC()},
		}},
		activeCount: 7,
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=1", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !reader.visibleOnly {
		t.Fatal("active count did not request visible-only operations")
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ActiveCount == nil || *payload.ActiveCount != 7 {
		t.Fatalf("active_count = %v, want 7", payload.ActiveCount)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "newest-terminal" {
		t.Fatalf("items = %+v", payload.Items)
	}
}

func TestHandleOperationsIncludesAuthoritativeZeroActiveCount(t *testing.T) {
	reader := &exactActiveCountReader{fakeBackgroundOperationReader: &fakeBackgroundOperationReader{}}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=1", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ActiveCount == nil || *payload.ActiveCount != 0 {
		t.Fatalf("active_count = %v, want explicit 0", payload.ActiveCount)
	}
}

func TestHandleOperationsFailsWhenExactActiveCountFails(t *testing.T) {
	reader := &exactActiveCountReader{
		fakeBackgroundOperationReader: &fakeBackgroundOperationReader{},
		countErr:                      errors.New("count failed"),
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}
