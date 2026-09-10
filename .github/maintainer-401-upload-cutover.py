from pathlib import Path


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    p = Path(path)
    text = p.read_text()
    if text.count(old) < count:
        raise SystemExit(f"{path}: replacement target not found")
    p.write_text(text.replace(old, new, count))


# Importer-only upload fallback is synchronous-only; real GooruLibrary uploads
# are routed to the durable producer before this handler.
p = Path("internal/serve/uploads.go")
text = p.read_text()
start = text.index("func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {")
end = text.index("\nfunc (s *Server) uploadRequestBodyLimit()", start)
replacement = '''func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
\tif r.Method != http.MethodPost {
\t\tw.Header().Set("Allow", http.MethodPost)
\t\twriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
\t\treturn
\t}
\timporter, ok := s.library.(UploadLibrary)
\tif s.library == nil || !ok {
\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "upload import service is not configured", nil)
\t\treturn
\t}
\tif !s.cfg.Uploads.Enabled {
\t\twriteError(w, http.StatusForbidden, "uploads_disabled", "uploads are disabled", nil)
\t\treturn
\t}
\tif !hasUploadTarget(s.cfg.Uploads.Targets) {
\t\twriteError(w, http.StatusForbidden, "uploads_disabled", "upload target is not configured", nil)
\t\treturn
\t}
\t// Real GooruLibrary uploads are selected into handleDurableUpload before
\t// reaching this fallback. Importer-only doubles and embedders cannot safely
\t// promise asynchronous recovery, so reject that capability before reading or
\t// staging the body instead of creating a second in-memory upload work model.
\tif PreferAsync(r) {
\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "asynchronous uploads require durable background operations", nil)
\t\treturn
\t}
\tif limit := s.uploadRequestBodyLimit(); limit > 0 {
\t\tr.Body = http.MaxBytesReader(w, r.Body, limit)
\t}
\ttags, saved, err := s.stageMultipartUpload(r)
\tif err != nil {
\t\twriteMultipartUploadError(w, err)
\t\treturn
\t}
\tif err := query.ValidateTags(tags); err != nil {
\t\tremoveSavedUploads(saved)
\t\twriteError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
\t\treturn
\t}
\tif len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
\t\tfileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
\t\twriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
\t\treturn
\t}
\tactivated, err := activateSavedReplacements(saved)
\tif err != nil {
\t\tremoveSavedUploads(saved)
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
\t\treturn
\t}
\tresponse, err := importer.ImportUploadedFiles(r.Context(), stagedUploads(saved), tags)
\tif err != nil {
\t\trollbackErr := rollbackSavedReplacements(activated)
\t\tremoveSavedUploads(saved)
\t\tif rollbackErr != nil {
\t\t\terr = fmt.Errorf("%w; replacement rollback failed: %v", err, rollbackErr)
\t\t}
\t\tif errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
\t\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
\t\t\treturn
\t\t}
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
\t\treturn
\t}
\tif err := settleSavedReplacements(activated, response); err != nil {
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to finalize uploaded files", nil)
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, response)
}'''
p.write_text(text[:start] + replacement + text[end:])

# Durable upload admission owns its own queue bound rather than borrowing the
# remaining legacy tag-job queue setting.
replace(
    "internal/serve/config.go",
    'type UploadsConfig struct {\n\tEnabled          bool           `yaml:"enabled"`\n\tTargets          []UploadTarget `yaml:"targets"`\n\tMaxFileSizeBytes int64          `yaml:"max_file_size_bytes"`',
    'type UploadsConfig struct {\n\tEnabled          bool           `yaml:"enabled"`\n\tTargets          []UploadTarget `yaml:"targets"`\n\tMaxFileSizeBytes int64          `yaml:"max_file_size_bytes"`\n\tMaxQueued        int            `yaml:"max_queued"`',
)
replace(
    "internal/serve/config.go",
    'Uploads: UploadsConfig{Enabled: false, ConflictPolicy: "rename", PreserveModTime: true},',
    'Uploads: UploadsConfig{Enabled: false, MaxQueued: 100, ConflictPolicy: "rename", PreserveModTime: true},',
)
replace(
    "internal/serve/config.go",
    '\tif cfg.Uploads.Enabled && !hasUploadTarget(cfg.Uploads.Targets) {',
    '\tif cfg.Uploads.MaxQueued <= 0 {\n\t\terrs = append(errs, errors.New("uploads.max_queued must be greater than zero"))\n\t}\n\tif cfg.Uploads.Enabled && !hasUploadTarget(cfg.Uploads.Targets) {',
)
replace(
    "internal/serve/uploads_durable.go",
    '\tif s.cfg.Jobs.MaxQueued > 0 {\n\t\treturn s.cfg.Jobs.MaxQueued\n\t}',
    '\tif s.cfg.Uploads.MaxQueued > 0 {\n\t\treturn s.cfg.Uploads.MaxQueued\n\t}',
)

# Extend the durable operations list endpoint with a bounded ID-batch form. This
# preserves the upload UI's one-request status window and includes completed
# durable results without reintroducing /jobs polling.
p = Path("internal/serve/background_operations.go")
text = p.read_text()
text = text.replace(
    "const defaultBackgroundOperationAPILimit = 100",
    "const (\n\tdefaultBackgroundOperationAPILimit = 100\n\tmaxBackgroundOperationStatusBatch = 64\n)",
    1,
)
marker = "\tlimit := defaultBackgroundOperationAPILimit\n"
batch = '''\tif rawIDs, ok := r.URL.Query()["id"]; ok {
\t\tids := make([]string, 0, len(rawIDs))
\t\tseen := make(map[string]struct{}, len(rawIDs))
\t\tfor _, rawID := range rawIDs {
\t\t\tid := strings.TrimSpace(rawID)
\t\t\tif id == "" {
\t\t\t\twriteError(w, http.StatusBadRequest, "invalid_request", "operation id must not be blank", nil)
\t\t\t\treturn
\t\t\t}
\t\t\tif _, exists := seen[id]; exists {
\t\t\t\tcontinue
\t\t\t}
\t\t\tseen[id] = struct{}{}
\t\t\tids = append(ids, id)
\t\t\tif len(ids) > maxBackgroundOperationStatusBatch {
\t\t\t\twriteError(w, http.StatusBadRequest, "invalid_request", "at most 64 operation ids may be requested", nil)
\t\t\t\treturn
\t\t\t}
\t\t}
\t\titems := make([]BackgroundOperationDTO, 0, len(ids))
\t\tfor _, id := range ids {
\t\t\toperation, found, err := s.backgroundOperations.GetBackgroundOperation(id)
\t\t\tif err != nil {
\t\t\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation", nil)
\t\t\t\treturn
\t\t\t}
\t\t\tif !found || !operation.Visible {
\t\t\t\tcontinue
\t\t\t}
\t\t\tdto := backgroundOperationDTO(operation)
\t\t\tif operation.Status == core.BackgroundWorkCompleted {
\t\t\t\tvar result json.RawMessage
\t\t\t\tfound, err := s.backgroundOperations.GetBackgroundOperationResult(id, &result)
\t\t\t\tif err != nil {
\t\t\t\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation result", nil)
\t\t\t\t\treturn
\t\t\t\t}
\t\t\t\tif found {
\t\t\t\t\tdto.Result = result
\t\t\t\t}
\t\t\t}
\t\t\titems = append(items, dto)
\t\t}
\t\twriteJSON(w, http.StatusOK, BackgroundOperationListResponse{Items: items})
\t\treturn
\t}
'''
if marker not in text:
    raise SystemExit("background_operations.go: list marker missing")
p.write_text(text.replace(marker, batch + marker, 1))

# Backend regressions.
p = Path("internal/serve/background_operations_test.go")
text = p.read_text().replace('import (\n', 'import (\n\t"fmt"\n', 1)
text += '''
func TestHandleOperationsBatchesRequestedIDsWithCompletedResults(t *testing.T) {
\tfinishedAt := time.Now().UTC()
\treader := &fakeBackgroundOperationReader{
\t\tbyID: map[string]core.BackgroundOperationState{
\t\t\t"operation-upload": {ID: "operation-upload", Kind: "upload_import", Visible: true, Status: core.BackgroundWorkCompleted, CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt},
\t\t\t"operation-running": {ID: "operation-running", Kind: "upload_import", Visible: true, Status: core.BackgroundWorkRunning, CreatedAt: finishedAt.Add(-time.Second)},
\t\t\t"operation-hidden": {ID: "operation-hidden", Kind: "thumbnail", Visible: false, Status: core.BackgroundWorkCompleted, CreatedAt: finishedAt},
\t\t},
\t\tresults: map[string]json.RawMessage{
\t\t\t"operation-upload": json.RawMessage(`{"affected_count":1,"files":[{"name":"a.jpg","size":1,"target_id":"default","status":"imported"}]}`),
\t\t},
\t}
\tserver := &Server{backgroundOperations: reader}
\trequest := httptest.NewRequest(http.MethodGet, "/api/v1/operations?id=operation-running&id=operation-upload&id=operation-hidden&id=missing", nil)
\tresponse := httptest.NewRecorder()

\tserver.handleOperations(response, request)

\tif response.Code != http.StatusOK {
\t\tt.Fatalf("status = %d body=%s", response.Code, response.Body.String())
\t}
\tvar payload BackgroundOperationListResponse
\tif err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
\t\tt.Fatalf("decode response: %v", err)
\t}
\tif len(payload.Items) != 2 || payload.Items[0].ID != "operation-running" || payload.Items[1].ID != "operation-upload" {
\t\tt.Fatalf("items = %+v", payload.Items)
\t}
\tvar result UploadImportResponse
\tif err := json.Unmarshal(payload.Items[1].Result, &result); err != nil {
\t\tt.Fatalf("decode upload result: %v", err)
\t}
\tif result.AffectedCount != 1 || len(result.Files) != 1 || result.Files[0].Name != "a.jpg" {
\t\tt.Fatalf("result = %+v", result)
\t}
}

func TestHandleOperationsRejectsOversizedIDBatch(t *testing.T) {
\tserver := &Server{backgroundOperations: &fakeBackgroundOperationReader{}}
\trequest := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
\tquery := request.URL.Query()
\tfor i := 0; i <= maxBackgroundOperationStatusBatch; i++ {
\t\tquery.Add("id", fmt.Sprintf("operation-%d", i))
\t}
\trequest.URL.RawQuery = query.Encode()
\tresponse := httptest.NewRecorder()

\tserver.handleOperations(response, request)

\tif response.Code != http.StatusBadRequest {
\t\tt.Fatalf("status = %d body=%s", response.Code, response.Body.String())
\t}
}
'''
p.write_text(text)

p = Path("internal/serve/uploads_test.go")
text = p.read_text()
text += '''
func TestUploadAsyncFallbackRejectsBeforeReadingBody(t *testing.T) {
\tserver := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})
\tbody := &uploadReadTracker{}
\trequest := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
\trequest.Header.Set("Content-Type", "multipart/form-data; boundary=unused")
\trequest.Header.Set("Prefer", "respond-async")
\tresponse := httptest.NewRecorder()

\tserver.Handler().ServeHTTP(response, request)

\tassertAPIError(t, response, http.StatusServiceUnavailable, "service_unavailable")
\tif body.read {
\t\tt.Fatal("async importer-only fallback read request body before rejecting unsupported durable capability")
\t}
}
'''
p.write_text(text)

replace(
    "internal/serve/config_test.go",
    '\tif cfg.Uploads.ConflictPolicy != "rename" {',
    '\tif cfg.Uploads.MaxQueued != 100 {\n\t\tt.Fatalf("unexpected upload queue default %d", cfg.Uploads.MaxQueued)\n\t}\n\tif cfg.Uploads.ConflictPolicy != "rename" {',
)
replace(
    "internal/serve/config_test.go",
    'func TestLoadConfigRejectsInvalidJobLimits(t *testing.T) {',
    'func TestLoadConfigRejectsInvalidUploadQueueLimit(t *testing.T) {\n\tcfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))\n\tcfg.Uploads.MaxQueued = 0\n\tif err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "uploads.max_queued") {\n\t\tt.Fatalf("expected upload queue limit validation error, got %v", err)\n\t}\n}\n\nfunc TestLoadConfigRejectsInvalidJobLimits(t *testing.T) {',
)

# Frontend upload polling/cancellation now uses durable operations only.
replace(
    "frontend/src/lib/api/operations.ts",
    '  error_message?: string;\n}',
    '  error_message?: string;\n  result?: unknown;\n}',
)
replace(
    "frontend/src/lib/api/operations.ts",
    'export function listBackgroundOperations(limit = 1000) {\n  const params = new URLSearchParams({ limit: String(limit) });\n  return operationRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);\n}\n',
    'export function listBackgroundOperations(limit = 1000) {\n  const params = new URLSearchParams({ limit: String(limit) });\n  return operationRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);\n}\n\nexport function listBackgroundOperationsByIDs(ids: string[]) {\n  const params = new URLSearchParams();\n  for (const id of ids) params.append("id", id);\n  return operationRequest<BackgroundOperationListResponse>(`/api/v1/operations?${params.toString()}`);\n}\n',
)
replace(
    "frontend/src/lib/api/operations.ts",
    '    finished_at: operation.finished_at,\n    error: operation.error_message\n',
    '    finished_at: operation.finished_at,\n    result: operation.result,\n    error: operation.error_message\n',
)
replace("frontend/src/lib/queries/jobs.ts", "import { ApiClient } from '$lib/api/client';\n", "")
replace(
    "frontend/src/lib/queries/jobs.ts",
    '  cancelBackgroundOperation,\n  listBackgroundOperations\n',
    '  cancelBackgroundOperation,\n  listBackgroundOperations,\n  listBackgroundOperationsByIDs\n',
)
p = Path("frontend/src/lib/queries/jobs.ts")
text = p.read_text()
start = text.index("async function fetchJobBatch(ids: string[]) {")
end = text.index("\nasync function fetchJobsPage", start)
text = text[:start] + '''async function fetchJobBatch(ids: string[]) {
  const response = await listBackgroundOperationsByIDs(ids);
  return { items: response.items.map(backgroundOperationAsJob) };
}
''' + text[end:]
text = text.replace(
    "// Upload result polling still uses the legacy JobManager until uploads become\n// durable-operation producers. Keep this mutation for callers outside the\n// durable operation history UI while that migration is incomplete.",
    "// Tag mutation is the final legacy JobManager producer. Keep finished-job\n// clearing only until tag mutation moves to durable operations in the next slice.",
)
p.write_text(text)

replace(
    "frontend/src/lib/api/client.ts",
    "import type { LibraryURLState } from '$lib/utils/appRoute';\n",
    "import type { LibraryURLState } from '$lib/utils/appRoute';\nimport type { BackgroundOperation } from './operations';\n",
)
replace(
    "frontend/src/lib/api/client.ts",
    '  ): Promise<Job | UploadImportResponse> {',
    '  ): Promise<BackgroundOperation | UploadImportResponse> {',
)
replace(
    "frontend/src/lib/api/client.ts",
    '    return uploadMultipart<Job | UploadImportResponse>',
    '    return uploadMultipart<BackgroundOperation | UploadImportResponse>',
)
replace(
    "frontend/src/lib/queries/library.ts",
    "import type { SavedSearch, SavedSearchRequest, UploadImportResponse, Job } from '$lib/api/types';\n",
    "import type { SavedSearch, SavedSearchRequest, UploadImportResponse } from '$lib/api/types';\nimport type { BackgroundOperation } from '$lib/api/operations';\n",
)
replace(
    "frontend/src/lib/queries/library.ts",
    '  return createMutation<Job | UploadImportResponse, Error, UploadVariables>',
    '  return createMutation<BackgroundOperation | UploadImportResponse, Error, UploadVariables>',
)
replace(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    "import type { Job, UploadImportResponse } from '$lib/api/types';\n",
    "import type { Job, UploadImportResponse } from '$lib/api/types';\nimport type { BackgroundOperation } from '$lib/api/operations';\n",
)
replace(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    'type UploadMutate = (variables: UploadVariables) => Promise<Job | UploadImportResponse>;',
    'type UploadMutate = (variables: UploadVariables) => Promise<BackgroundOperation | UploadImportResponse>;',
)
replace(
    "frontend/src/lib/state/uploadWorkflow.svelte.ts",
    '          let response: Job | UploadImportResponse;',
    '          let response: BackgroundOperation | UploadImportResponse;',
)

# OpenAPI contract: document durable operations and make upload 202 truthful.
p = Path("docs/openapi.yaml")
text = p.read_text()
operations_paths = '''  /operations:
    get:
      summary: List durable background operations.
      security:
        - sessionAuth: []
      parameters:
        - name: id
          in: query
          required: false
          description: Specific durable operation IDs to return. May be repeated; at most 64 unique IDs are accepted. Completed results are included for this bounded status-batch form.
          schema:
            type: array
            maxItems: 64
            items:
              type: string
          style: form
          explode: true
        - name: limit
          in: query
          required: false
          schema:
            type: integer
            minimum: 1
            maximum: 1000
            default: 100
      responses:
        "200":
          description: Visible durable operations matching the request.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/BackgroundOperationListResponse"
        "400":
          $ref: "#/components/responses/BadRequest"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
  /operations/{id}:
    get:
      summary: Get one durable background operation.
      security:
        - sessionAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Durable operation state, including a structured result when completed and available.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/BackgroundOperation"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
        "404":
          $ref: "#/components/responses/NotFound"
    delete:
      summary: Cancel an active durable background operation.
      security:
        - sessionAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - $ref: "#/components/parameters/CSRF"
      responses:
        "202":
          description: Operation canceled.
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/BackgroundOperation"
        "401":
          $ref: "#/components/responses/Unauthorized"
        "403":
          $ref: "#/components/responses/Forbidden"
        "404":
          $ref: "#/components/responses/NotFound"
'''
if "  /jobs:\n" not in text:
    raise SystemExit("openapi jobs path marker missing")
text = text.replace("  /jobs:\n", operations_paths + "  /jobs:\n", 1)
uploads_start = text.index("  /uploads:\n")
tags_start = text.index("  /tags:\n", uploads_start)
upload_block = text[uploads_start:tags_start]
old_async = '        "202":\n          $ref: "#/components/responses/AsyncJob"'
if old_async not in upload_block:
    raise SystemExit("upload AsyncJob response marker missing")
upload_block = upload_block.replace(
    old_async,
    '        "202":\n          $ref: "#/components/responses/AsyncOperation"',
    1,
)
text = text[:uploads_start] + upload_block + text[tags_start:]
text = text.replace(
    "      description: Set to respond-async to enqueue the mutation and poll the returned job.",
    "      description: Set to respond-async to accept the mutation asynchronously; poll the returned durable operation or legacy job as documented by the endpoint.",
    1,
)
text = text.replace(
    "    AsyncJob:\n",
    '''    AsyncOperation:
      description: Mutation was accepted as a durable background operation.
      content:
        application/json:
          schema:
            $ref: "#/components/schemas/BackgroundOperation"
    AsyncJob:
''',
    1,
)
background_schemas = '''    BackgroundOperation:
      type: object
      required: [id, kind, status, progress_total, progress_completed, progress_failed, created_at]
      properties:
        id:
          type: string
        kind:
          type: string
        status:
          type: string
          enum: [pending, running, completed, failed, canceled]
        progress_total:
          type: integer
          format: int64
          minimum: 0
        progress_completed:
          type: integer
          format: int64
          minimum: 0
        progress_failed:
          type: integer
          format: int64
          minimum: 0
        created_at:
          type: string
          format: date-time
        started_at:
          type: string
          format: date-time
        finished_at:
          type: string
          format: date-time
        error_code:
          type: string
        error_message:
          type: string
        result: {}
    BackgroundOperationListResponse:
      type: object
      required: [items]
      properties:
        items:
          type: array
          items:
            $ref: "#/components/schemas/BackgroundOperation"
'''
if "    Job:\n" not in text:
    raise SystemExit("openapi Job schema marker missing")
text = text.replace("    Job:\n", background_schemas + "    Job:\n", 1)
p.write_text(text)
