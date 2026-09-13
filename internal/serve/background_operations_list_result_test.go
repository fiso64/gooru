package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "gooru.local/gooru"
)

func TestHandleOperationsIncludesCompletedStructuredResult(t *testing.T) {
	finishedAt := time.Now().UTC()
	reader := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{{
			ID: "operation-tag", Kind: core.BackgroundTagMutationOperationKind, Visible: true,
			Status: core.BackgroundWorkCompleted, ProgressTotal: 1, ProgressCompleted: 1,
			CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt,
		}},
		results: map[string]json.RawMessage{
			"operation-tag": json.RawMessage(`{"affected_count":7}`),
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=20", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %+v", payload.Items)
	}
	var result struct {
		AffectedCount int `json:"affected_count"`
	}
	if err := json.Unmarshal(payload.Items[0].Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.AffectedCount != 7 {
		t.Fatalf("affected count = %d, want 7; payload=%+v", result.AffectedCount, payload.Items[0])
	}
}

func TestHandleOperationsPrefersDirectUploadResultAfterAttachedTasksIncreaseTotal(t *testing.T) {
	finishedAt := time.Now().UTC()
	operationID := "operation-direct-upload"
	result := UploadImportResponse{
		Files: []UploadedFileDTO{
			{Name: "a.jpg", TargetID: "default", Status: "imported"},
			{Name: "b.jpg", TargetID: "default", Status: "imported"},
		},
		AffectedCount: 2,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	base := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{{
			ID: operationID, Kind: backgroundUploadImportOperationKind, Visible: true,
			Status: core.BackgroundWorkCompleted, ProgressTotal: 3, ProgressCompleted: 3,
			CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt,
		}},
		results: map[string]json.RawMessage{operationID: encoded},
	}
	reader := &fakeSegmentedUploadReader{
		fakeBackgroundOperationReader: base,
		tasks:                           map[string]core.BackgroundTaskState{},
		taskCheckpoints:                 map[string]backgroundUploadCheckpoint{},
		taskResults:                     map[string]UploadImportResponse{},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=20", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %+v", payload.Items)
	}
	var got UploadImportResponse
	if err := json.Unmarshal(payload.Items[0].Result, &got); err != nil {
		t.Fatalf("decode upload result: %v", err)
	}
	if got.AffectedCount != 2 || len(got.Files) != 2 {
		t.Fatalf("upload result = %+v", got)
	}
}
