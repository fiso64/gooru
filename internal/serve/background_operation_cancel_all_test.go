package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "gooru.local/gooru"
)

type fakeActiveOperationLister struct {
	fakeBackgroundOperationReader
	ids []string
}

func (f *fakeActiveOperationLister) ListActiveBackgroundOperationIDs(visibleOnly bool) ([]string, error) {
	if !visibleOnly {
		return nil, nil
	}
	return append([]string(nil), f.ids...), nil
}

func TestHandleCancelAllOperationsCancelsCompleteActiveSnapshot(t *testing.T) {
	createdAt := time.Now().UTC()
	reader := &fakeActiveOperationLister{
		fakeBackgroundOperationReader: fakeBackgroundOperationReader{
			byID: map[string]core.BackgroundOperationState{
				"one": {ID: "one", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkPending, CreatedAt: createdAt},
				"two": {ID: "two", Kind: "library-delete", Visible: true, Status: core.BackgroundWorkRunning, CreatedAt: createdAt},
			},
			cancelOK: true,
		},
		ids: []string{"one", "two"},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/cancel-all", nil)
	response := httptest.NewRecorder()

	server.handleCancelAllOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationCancelAllResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Canceled != 2 {
		t.Fatalf("canceled = %d, want 2", payload.Canceled)
	}
	for _, id := range reader.ids {
		if reader.byID[id].Status != core.BackgroundWorkCanceled {
			t.Fatalf("operation %s status = %s", id, reader.byID[id].Status)
		}
	}
}
