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

type pagingBackgroundOperationReader struct {
	*fakeBackgroundOperationReader
	totalCount int
	totalErr   error
}

func (r *pagingBackgroundOperationReader) CountBackgroundOperations(visibleOnly bool) (int, error) {
	if !visibleOnly {
		return 0, errors.New("expected visible-only count")
	}
	if r.totalErr != nil {
		return 0, r.totalErr
	}
	return r.totalCount, nil
}

func TestHandleOperationsPropagatesOffsetAndReturnsTotalCount(t *testing.T) {
	createdAt := time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC)
	base := &fakeBackgroundOperationReader{operations: []core.BackgroundOperationState{{
		ID: "operation-page", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkCompleted, CreatedAt: createdAt,
	}}}
	reader := &pagingBackgroundOperationReader{fakeBackgroundOperationReader: base, totalCount: 137}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=25&offset=50", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if !base.listOptions.VisibleOnly || base.listOptions.Limit != 25 || base.listOptions.Offset != 50 {
		t.Fatalf("list options = %+v", base.listOptions)
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TotalCount == nil || *payload.TotalCount != 137 {
		t.Fatalf("total count = %v, want 137", payload.TotalCount)
	}
}

func TestHandleOperationsRejectsNegativeOffset(t *testing.T) {
	server := &Server{backgroundOperations: &fakeBackgroundOperationReader{}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?offset=-1", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}
