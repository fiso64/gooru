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

type fakeBackgroundOperationSummaryReader struct {
	*fakeBackgroundOperationReader
	summaries map[string]core.BackgroundOperationResultSummary
	ids       []string
	err       error
}

func (f *fakeBackgroundOperationSummaryReader) GetBackgroundOperationResultSummaries(ids []string) (map[string]core.BackgroundOperationResultSummary, error) {
	f.ids = append([]string(nil), ids...)
	if f.err != nil {
		return nil, f.err
	}
	return f.summaries, nil
}

func TestHandleOperationsUsesCompactCompletedSummaryWithoutStructuredResult(t *testing.T) {
	finishedAt := time.Now().UTC()
	base := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{{
			ID: "operation-tag", Kind: core.BackgroundTagMutationOperationKind, Visible: true,
			Status: core.BackgroundWorkCompleted, ProgressTotal: 1, ProgressCompleted: 1,
			CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt,
		}},
		results:   map[string]json.RawMessage{"operation-tag": json.RawMessage(`{"affected_count":7}`)},
		resultErr: errors.New("paginated list must not load structured result"),
	}
	affected, failed := int64(7), int64(0)
	reader := &fakeBackgroundOperationSummaryReader{
		fakeBackgroundOperationReader: base,
		summaries: map[string]core.BackgroundOperationResultSummary{
			"operation-tag": {Outcome: "success", AffectedCount: &affected, FailedCount: &failed},
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=20", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			Result        json.RawMessage `json:"result"`
			Outcome       string          `json:"outcome"`
			AffectedCount *int64          `json:"affected_count"`
			FailedCount   *int64          `json:"failed_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %+v", payload.Items)
	}
	item := payload.Items[0]
	if len(item.Result) != 0 {
		t.Fatalf("paginated result unexpectedly present: %s", item.Result)
	}
	if item.Outcome != "success" || item.AffectedCount == nil || *item.AffectedCount != 7 || item.FailedCount == nil || *item.FailedCount != 0 {
		t.Fatalf("compact summary = %+v", item)
	}
	if len(reader.ids) != 1 || reader.ids[0] != "operation-tag" {
		t.Fatalf("summary ids = %v", reader.ids)
	}
}

func TestHandleOperationsSkipsCompletedUploadResultAndRecoveryCheckpoint(t *testing.T) {
	finishedAt := time.Now().UTC()
	operationID := "operation-direct-upload"
	base := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{{
			ID: operationID, Kind: backgroundUploadImportOperationKind, Visible: true,
			Status: core.BackgroundWorkCompleted, ProgressTotal: 3, ProgressCompleted: 3,
			CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt,
		}},
		results: map[string]json.RawMessage{
			operationID: json.RawMessage(`{"files":[{"name":"a.jpg","status":"imported"},{"name":"b.jpg","status":"error"}]}`),
		},
		resultErr: errors.New("paginated list must not load upload result"),
		checkpoints: map[string]backgroundUploadCheckpoint{
			operationID: {Phase: backgroundUploadPhaseImported, FileTotal: 999, FilesCompleted: 999},
		},
	}
	affected, failed := int64(1), int64(1)
	reader := &fakeBackgroundOperationSummaryReader{
		fakeBackgroundOperationReader: base,
		summaries: map[string]core.BackgroundOperationResultSummary{
			operationID: {Outcome: "partial_success", AffectedCount: &affected, FailedCount: &failed},
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations?limit=20", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Items []struct {
			Result        json.RawMessage `json:"result"`
			Outcome       string          `json:"outcome"`
			AffectedCount *int64          `json:"affected_count"`
			FailedCount   *int64          `json:"failed_count"`
			ProgressTotal int64           `json:"progress_total"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %+v", payload.Items)
	}
	item := payload.Items[0]
	if len(item.Result) != 0 {
		t.Fatalf("upload result unexpectedly present: %s", item.Result)
	}
	if item.Outcome != "partial_success" || item.AffectedCount == nil || *item.AffectedCount != 1 || item.FailedCount == nil || *item.FailedCount != 1 {
		t.Fatalf("upload summary = %+v", item)
	}
	if item.ProgressTotal != 3 {
		t.Fatalf("completed list read recovery checkpoint: progress_total=%d, want durable scalar 3", item.ProgressTotal)
	}
}
