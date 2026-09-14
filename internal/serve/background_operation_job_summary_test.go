package serve

import (
	"encoding/json"
	"testing"

	core "gooru.local/gooru"
)

func decodeOperationSummary(t *testing.T, dto BackgroundOperationDTO) struct {
	Outcome       string `json:"outcome"`
	AffectedCount *int64 `json:"affected_count"`
	FailedCount   *int64 `json:"failed_count"`
} {
	t.Helper()
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var summary struct {
		Outcome       string `json:"outcome"`
		AffectedCount *int64 `json:"affected_count"`
		FailedCount   *int64 `json:"failed_count"`
	}
	if err := json.Unmarshal(encoded, &summary); err != nil {
		t.Fatal(err)
	}
	return summary
}

func TestBackgroundOperationJobSummaryReportsPartialUpload(t *testing.T) {
	result, err := json.Marshal(UploadImportResponse{Files: []UploadedFileDTO{
		{Name: "ok.jpg", Status: "imported"},
		{Name: "large.jpg", Status: "error", Error: "too large"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	summary := decodeOperationSummary(t, BackgroundOperationDTO{
		Kind: backgroundUploadImportOperationKind, Status: core.BackgroundWorkCompleted, Result: result,
	})
	if summary.Outcome != "partial_success" || summary.AffectedCount == nil || *summary.AffectedCount != 1 || summary.FailedCount == nil || *summary.FailedCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestBackgroundOperationJobSummaryReportsAllFailedUploadAsError(t *testing.T) {
	result, err := json.Marshal(UploadImportResponse{Files: []UploadedFileDTO{
		{Name: "one.jpg", Status: "error"},
		{Name: "two.jpg", Status: "error"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	summary := decodeOperationSummary(t, BackgroundOperationDTO{
		Kind: backgroundUploadImportOperationKind, Status: core.BackgroundWorkCompleted, Result: result,
	})
	if summary.Outcome != "error" || summary.AffectedCount == nil || *summary.AffectedCount != 0 || summary.FailedCount == nil || *summary.FailedCount != 2 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestBackgroundOperationJobSummaryUsesMatchedFilesForTagMutation(t *testing.T) {
	summary := decodeOperationSummary(t, BackgroundOperationDTO{
		Kind: core.BackgroundTagMutationOperationKind, Status: core.BackgroundWorkCompleted,
		Result: json.RawMessage(`{"matched_files":5,"affected_count":15}`),
	})
	if summary.Outcome != "success" || summary.AffectedCount == nil || *summary.AffectedCount != 5 || summary.FailedCount == nil || *summary.FailedCount != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestBackgroundOperationJobSummaryReportsOversizedProducerFailure(t *testing.T) {
	summary := decodeOperationSummary(t, BackgroundOperationDTO{
		Kind: backgroundUploadImportOperationKind, Status: core.BackgroundWorkFailed,
		Stage: "importing", ProgressTotal: 1, ErrorCode: "payload_too_large",
	})
	if summary.Outcome != "error" || summary.AffectedCount == nil || *summary.AffectedCount != 0 || summary.FailedCount == nil || *summary.FailedCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
}
