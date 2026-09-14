from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected snippet missing in {path}")
    p.write_text(text.replace(old, new, 1))


replace(
    "internal/serve/uploads_durable_failure.go",
    '''func failDurableUploadProducer(operations durableUploadOperationStore, operationID string) {
\tif failures, ok := any(operations).(durableUploadFailureStore); ok {
\t\t_, _ = failures.FailBackgroundOperationWithCleanupTask(
\t\t\toperationID,
\t\t\t"upload_request_failed",
\t\t\t"upload request failed before durable import",
\t\t\tbackgroundUploadCleanupTaskRequest(operationID),
\t\t)
\t\treturn
\t}
\t_, _ = operations.CancelBackgroundOperation(operationID)
}
''',
    '''func failDurableUploadProducer(operations durableUploadOperationStore, operationID string) {
\tfailDurableUploadProducerWithError(operations, operationID, "upload_request_failed", "upload request failed before durable import")
}

func failDurableUploadProducerWithError(operations durableUploadOperationStore, operationID, errorCode, errorMessage string) {
\tif failures, ok := any(operations).(durableUploadFailureStore); ok {
\t\t_, _ = failures.FailBackgroundOperationWithCleanupTask(
\t\t\toperationID,
\t\t\terrorCode,
\t\t\terrorMessage,
\t\t\tbackgroundUploadCleanupTaskRequest(operationID),
\t\t)
\t\treturn
\t}
\t_, _ = operations.CancelBackgroundOperation(operationID)
}
''',
)

replace(
    "internal/serve/uploads_durable.go",
    '''\tif len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
\t\tfileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
\t\twriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
\t\treturn
\t}
''',
    '''\tif len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
\t\tfileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
\t\tfailDurableUploadProducerWithError(operations, operation.ID, "payload_too_large", fileErr.Error())
\t\twriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
\t\treturn
\t}
''',
)

replace(
    "internal/serve/background_operation_job_summary.go",
    '''\t\tif dto.Kind == backgroundUploadImportOperationKind && dto.ErrorCode == "payload_too_large" && dto.Stage == "importing" && dto.ProgressTotal > 0 {
\t\t\tzero := int64(0)
\t\t\tfailed := dto.ProgressTotal
\t\t\treturn outcome, &zero, &failed
\t\t}
''',
    '''\t\tif dto.Kind == backgroundUploadImportOperationKind && dto.ErrorCode == "payload_too_large" {
\t\t\tzero := int64(0)
\t\t\tone := int64(1)
\t\t\treturn outcome, &zero, &one
\t\t}
''',
)

replace(
    "internal/serve/background_operation_job_summary_test.go",
    '''\t\tKind: backgroundUploadImportOperationKind, Status: core.BackgroundWorkFailed,
\t\tStage: "importing", ProgressTotal: 1, ErrorCode: "payload_too_large",
''',
    '''\t\tKind: backgroundUploadImportOperationKind, Status: core.BackgroundWorkFailed,
\t\tErrorCode: "payload_too_large",
''',
)

replace(
    "internal/serve/uploads_durable_failure_test.go",
    '''\t\tItems []struct {
\t\t\tID      string `json:"id"`
\t\t\tStatus  string `json:"status"`
\t\t\tOutcome string `json:"outcome"`
\t\t\tError   string `json:"error_message"`
\t\t} `json:"items"`
''',
    '''\t\tItems []struct {
\t\t\tID            string `json:"id"`
\t\t\tStatus        string `json:"status"`
\t\t\tOutcome       string `json:"outcome"`
\t\t\tAffectedCount *int64 `json:"affected_count"`
\t\t\tFailedCount   *int64 `json:"failed_count"`
\t\t\tErrorCode     string `json:"error_code"`
\t\t\tError         string `json:"error_message"`
\t\t} `json:"items"`
''',
)

replace(
    "internal/serve/uploads_durable_failure_test.go",
    '''\tif job.Status != string(core.BackgroundWorkFailed) || job.Outcome != "error" {
\t\tt.Fatalf("oversized job = %+v, want failed/error", job)
\t}
\tif job.Error == "" {
\t\tt.Fatalf("oversized job has no durable error detail: %+v", job)
\t}
''',
    '''\tif job.Status != string(core.BackgroundWorkFailed) || job.Outcome != "error" {
\t\tt.Fatalf("oversized job = %+v, want failed/error", job)
\t}
\tif job.ErrorCode != "payload_too_large" || job.Error == "" {
\t\tt.Fatalf("oversized job has wrong durable error detail: %+v", job)
\t}
\tif job.AffectedCount == nil || *job.AffectedCount != 0 || job.FailedCount == nil || *job.FailedCount != 1 {
\t\tt.Fatalf("oversized job counts = %+v, want affected=0 failed=1", job)
\t}
''',
)
