from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected snippet missing in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))


# Checkpoint model: transport receive is an explicit phase, while existing file
# counters remain the source of truth for row-level import progress.
replace(
    "internal/serve/background_uploads.go",
    '\tbackgroundUploadInputVersion   = 1\n\tbackgroundUploadPhaseStaged    = "staged"\n',
    '\tbackgroundUploadInputVersion   = 1\n\tbackgroundUploadPhaseReceiving = "receiving"\n\tbackgroundUploadPhaseStaged    = "staged"\n',
)
replace(
    "internal/serve/background_uploads.go",
    '\tFilesCompleted       int                                     `json:"files_completed,omitempty"`\n\tFilesCompletedPrefix int                                     `json:"files_completed_prefix,omitempty"`\n}',
    '\tFilesCompleted       int                                     `json:"files_completed,omitempty"`\n\tFilesCompletedPrefix int                                     `json:"files_completed_prefix,omitempty"`\n\tTransportBytesTotal    int64                                   `json:"transport_bytes_total,omitempty"`\n\tTransportBytesReceived int64                                   `json:"transport_bytes_received,omitempty"`\n}',
)
replace(
    "internal/serve/background_uploads.go",
    "func backgroundUploadInitialCheckpoint(fileTotal ...int) backgroundUploadCheckpoint {",
    '''func backgroundUploadReceivingCheckpoint(total, received int64) backgroundUploadCheckpoint {
\tif total < 0 {
\t\ttotal = 0
\t}
\tif received < 0 {
\t\treceived = 0
\t}
\tif total > 0 && received > total {
\t\treceived = total
\t}
\treturn backgroundUploadCheckpoint{
\t\tPhase:                  backgroundUploadPhaseReceiving,
\t\tTransportBytesTotal:    total,
\t\tTransportBytesReceived: received,
\t}
}

func backgroundUploadInitialCheckpoint(fileTotal ...int) backgroundUploadCheckpoint {''',
)

# Producer: persist a receiving checkpoint and expose the operation before the
# request body is consumed. The body wrapper publishes bounded byte progress.
replace(
    "internal/serve/uploads_durable.go",
    '''type durableUploadOperationStore interface {
\tbackgroundOperationReader
\tCreateBackgroundOperation(core.BackgroundOperationRequest) (core.BackgroundOperation, error)
\tAttachBackgroundTaskAndRevealOperation(string, any, core.BackgroundTaskRequest) (core.BackgroundTask, error)
}''',
    '''type durableUploadOperationStore interface {
\tbackgroundOperationReader
\tCreateBackgroundOperation(core.BackgroundOperationRequest) (core.BackgroundOperation, error)
\tSetBackgroundOperationCheckpoint(string, any) error
\tSetBackgroundOperationVisible(string, bool) error
\tAttachBackgroundTaskAndRevealOperation(string, any, core.BackgroundTaskRequest) (core.BackgroundTask, error)
}''',
)
replace(
    "internal/serve/uploads_durable.go",
    '''type durableUploadCleanupStore interface {
\tGetBackgroundOperationTask(string) (core.BackgroundTaskState, bool, error)
\tGetBackgroundOperationCheckpoint(string, any) (bool, error)
}''',
    '''type durableUploadTaskReader interface {
\tGetBackgroundOperationTask(string) (core.BackgroundTaskState, bool, error)
}

type durableUploadCleanupStore interface {
\tdurableUploadTaskReader
\tGetBackgroundOperationCheckpoint(string, any) (bool, error)
}''',
)
replace(
    "internal/serve/uploads_durable.go",
    '''\tattached := false
\tdefer func() {
\t\tif !attached {
\t\t\t_, _ = operations.CancelBackgroundOperation(operation.ID)
\t\t}
\t}()

\ttags, saved, err := s.stageMultipartUpload(r)''',
    '''\tattached := false
\tdefer func() {
\t\tif !attached {
\t\t\t_, _ = operations.CancelBackgroundOperation(operation.ID)
\t\t}
\t}()

\tif err := operations.SetBackgroundOperationCheckpoint(operation.ID, backgroundUploadReceivingCheckpoint(r.ContentLength, 0)); err != nil {
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to initialize upload progress", nil)
\t\treturn
\t}
\tif err := operations.SetBackgroundOperationVisible(operation.ID, true); err != nil {
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to publish upload operation", nil)
\t\treturn
\t}
\tr.Body = newUploadReceivingProgressReadCloser(r.Body, operation.ID, r.ContentLength, operations)

\ttags, saved, err := s.stageMultipartUpload(r)''',
)
replace(
    "internal/serve/uploads_durable.go",
    '''\tif err != nil {
\t\twriteMultipartUploadError(w, err)
\t\treturn
\t}''',
    '''\tif err != nil {
\t\tif errors.Is(err, errUploadReceivingCanceled) {
\t\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "upload was canceled", nil)
\t\t\treturn
\t\t}
\t\twriteMultipartUploadError(w, err)
\t\treturn
\t}''',
)
replace(
    "internal/serve/uploads_durable.go",
    '''\tstore, ok := s.backgroundOperations.(durableUploadCancellationStore)
\tif !ok {
\t\treturn s.backgroundOperations.CancelBackgroundOperation(operationID)
\t}
\tresult, err := store.CancelBackgroundOperationWithCleanupTask(operationID, backgroundUploadCleanupTaskRequest(operationID))''',
    '''\t// Receiving uploads are visible before their import child exists. There are
\t// no durable external side effects to compensate yet; the request handler
\t// owns partial staging cleanup while unwinding from cancellation.
\tif taskStore, ok := s.backgroundOperations.(durableUploadTaskReader); ok {
\t\tif _, found, taskErr := taskStore.GetBackgroundOperationTask(operationID); taskErr != nil {
\t\t\treturn false, taskErr
\t\t} else if !found {
\t\t\treturn s.backgroundOperations.CancelBackgroundOperation(operationID)
\t\t}
\t}

\tstore, ok := s.backgroundOperations.(durableUploadCancellationStore)
\tif !ok {
\t\treturn s.backgroundOperations.CancelBackgroundOperation(operationID)
\t}
\tresult, err := store.CancelBackgroundOperationWithCleanupTask(operationID, backgroundUploadCleanupTaskRequest(operationID))''',
)

# Attachment remains atomic but accepts the already-visible receiving upload.
replace(
    "internal/database/durable_work_attachment.go",
    '''// AttachBackgroundTaskAndRevealOperation atomically persists producer recovery
// state, attaches the first durable child task to a hidden admitted operation,
// and reveals that operation to consumers. A failed attachment rolls all three
// mutations back so startup recovery can safely release the still-hidden
// reservation.''',
    '''// AttachBackgroundTaskAndRevealOperation atomically persists producer recovery
// state, attaches the first durable child task to an admitted operation, and
// ensures the operation is visible. Most producers attach while hidden; upload
// operations may already be visible while their HTTP body is being received.''',
)
replace(
    "internal/database/durable_work_attachment.go",
    '''\tif visible != 0 || status != string(BackgroundWorkPending) || attached != 0 {
\t\treturn BackgroundTask{}, errors.New("background operation is not an unattached hidden pending reservation")
\t}''',
    '''\tif (visible != 0 && visible != 1) || status != string(BackgroundWorkPending) || attached != 0 {
\t\treturn BackgroundTask{}, errors.New("background operation is not an unattached pending reservation")
\t}''',
)
replace(
    "gooru/background.go",
    '''// AttachBackgroundTaskAndRevealOperation finishes producer admission after
// expensive staging. The recovery checkpoint, first child task, and visibility
// transition commit atomically, so a crash cannot leave a half-attached hidden
// operation or expose work without its persisted recovery state.''',
    '''// AttachBackgroundTaskAndRevealOperation finishes producer admission after
// expensive staging. The recovery checkpoint, first child task, and final
// visibility state commit atomically. Producers normally attach hidden work;
// uploads may already be visible while their request body is still arriving.''',
)

# Startup recovery must cover visible receiving uploads that died before attach.
p = Path("internal/database/durable_work_recovery.go")
p.write_text(p.read_text() + '''

// CancelUnattachedBackgroundOperations cancels pending operations of one kind
// that have no durable child task, regardless of visibility. Visible upload
// receiving operations use this at startup because a process death can occur
// after publication but before multipart staging attaches durable work.
func (s *Store) CancelUnattachedBackgroundOperations(kind string) (int64, error) {
\tif s == nil || s.DB == nil {
\t\treturn 0, errors.New("background operation store is required")
\t}
\tkind = strings.TrimSpace(kind)
\tif kind == "" {
\t\treturn 0, errors.New("background operation kind is required")
\t}
\tfinishedAt := workTimeValue(time.Now().UTC())
\tresult, err := s.DB.Exec(`
\t\tUPDATE background_operations
\t\tSET status = 'canceled', finished_at = ?,
\t\t    error_code = 'producer_interrupted',
\t\t    error_message = 'producer interrupted before durable work was attached'
\t\tWHERE kind = ?
\t\t  AND status = 'pending'
\t\t  AND NOT EXISTS (
\t\t\tSELECT 1 FROM background_tasks
\t\t\tWHERE background_tasks.operation_id = background_operations.id
\t\t  )
\t`, finishedAt, kind)
\tif err != nil {
\t\treturn 0, fmt.Errorf("cancel unattached background operations: %w", err)
\t}
\tcount, err := result.RowsAffected()
\tif err != nil {
\t\treturn 0, fmt.Errorf("cancel unattached background operations rows affected: %w", err)
\t}
\treturn count, nil
}
''')
replace(
    "gooru/background_checkpoint.go",
    '''func (c *Client) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
\treturn c.store.CancelUnattachedHiddenBackgroundOperations(kind)
}''',
    '''func (c *Client) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
\treturn c.store.CancelUnattachedHiddenBackgroundOperations(kind)
}

// CancelUnattachedBackgroundOperations releases pre-crash producer work that
// was already visible before its durable child task could be attached.
func (c *Client) CancelUnattachedBackgroundOperations(kind string) (int64, error) {
\treturn c.store.CancelUnattachedBackgroundOperations(kind)
}''',
)
replace(
    "internal/serve/background_operation_recovery.go",
    '''type backgroundOperationReservationRecovery interface {
\tCancelUnattachedHiddenBackgroundOperations(string) (int64, error)
}''',
    '''type backgroundOperationReservationRecovery interface {
\tCancelUnattachedHiddenBackgroundOperations(string) (int64, error)
\tCancelUnattachedBackgroundOperations(string) (int64, error)
}''',
)
replace(
    "internal/serve/background_operation_recovery.go",
    '''\tfor _, item := range []struct {
\t\tkind string
\t\tname string
\t}{
\t\t{kind: backgroundUploadImportOperationKind, name: "upload"},
\t\t{kind: core.BackgroundTagMutationOperationKind, name: "tag mutation"},
\t} {
\t\tif _, err := recovery.CancelUnattachedHiddenBackgroundOperations(item.kind); err != nil {
\t\t\treturn fmt.Errorf("recover %s background operation reservations: %w", item.name, err)
\t\t}
\t}''',
    '''\tif _, err := recovery.CancelUnattachedBackgroundOperations(backgroundUploadImportOperationKind); err != nil {
\t\treturn fmt.Errorf("recover upload background operation reservations: %w", err)
\t}
\tif _, err := recovery.CancelUnattachedHiddenBackgroundOperations(core.BackgroundTagMutationOperationKind); err != nil {
\t\treturn fmt.Errorf("recover tag mutation background operation reservations: %w", err)
\t}''',
)

# API: keep raw file counters stable and add a separate overall fraction.
replace(
    "internal/serve/background_operations.go",
    '\tProgressFailed          int64                     `json:"progress_failed"`\n\tCreatedAt',
    '\tProgressFailed          int64                     `json:"progress_failed"`\n\tProgress                *float64                  `json:"progress,omitempty"`\n\tCreatedAt',
)
replace(
    "internal/serve/background_operations.go",
    '''\tvar checkpoint backgroundUploadCheckpoint
\tfound, err := reader.GetBackgroundOperationCheckpoint(operation.ID, &checkpoint)
\tif err != nil || !found || checkpoint.FileTotal <= 0 {
\t\treturn dto
\t}
\ttotal := int64(checkpoint.FileTotal)''',
    '''\tvar checkpoint backgroundUploadCheckpoint
\tfound, err := reader.GetBackgroundOperationCheckpoint(operation.ID, &checkpoint)
\tif err != nil || !found {
\t\treturn dto
\t}
\tif checkpoint.Phase == backgroundUploadPhaseReceiving {
\t\tif checkpoint.TransportBytesTotal > 0 {
\t\t\treceived := checkpoint.TransportBytesReceived
\t\t\tif received < 0 {
\t\t\t\treceived = 0
\t\t\t}
\t\t\tif received > checkpoint.TransportBytesTotal {
\t\t\t\treceived = checkpoint.TransportBytesTotal
\t\t\t}
\t\t\tprogress := 0.5 * float64(received) / float64(checkpoint.TransportBytesTotal)
\t\t\tdto.Progress = &progress
\t\t}
\t\treturn dto
\t}
\tif checkpoint.FileTotal <= 0 {
\t\treturn dto
\t}
\ttotal := int64(checkpoint.FileTotal)''',
)
replace(
    "internal/serve/background_operations.go",
    '''\tdto.ProgressTotal = total
\tdto.ProgressCompleted = completed
\tdto.ProgressCompletedPrefix = completedPrefix
\treturn dto
}''',
    '''\tdto.ProgressTotal = total
\tdto.ProgressCompleted = completed
\tdto.ProgressCompletedPrefix = completedPrefix
\tprogress := 0.5 + 0.5*float64(completed)/float64(total)
\tif operation.Status == core.BackgroundWorkCompleted {
\t\tprogress = 1
\t}
\tif progress > 1 {
\t\tprogress = 1
\t}
\tdto.Progress = &progress
\treturn dto
}''',
)

# Frontend adapter/UI: uploads without a measured overall fraction are explicitly
# indeterminate, so JobRow omits both bar and percentage.
replace(
    "frontend/src/lib/api/operations.ts",
    "  progress_failed: number;\n  created_at: string;",
    "  progress_failed: number;\n  progress?: number;\n  created_at: string;",
)
replace(
    "frontend/src/lib/api/operations.ts",
    '''  const progress = total > 0
    ? Math.max(0, Math.min(1, completed / total))
    : operation.status === 'completed' || operation.status === 'failed' || operation.status === 'canceled'
      ? 1
      : 0;''',
    '''  const fallbackProgress = total > 0
    ? Math.max(0, Math.min(1, completed / total))
    : operation.status === 'completed' || operation.status === 'failed' || operation.status === 'canceled'
      ? 1
      : undefined;
  const progress = operation.progress !== undefined
    ? Math.max(0, Math.min(1, operation.progress))
    : operation.kind === 'upload_import'
      ? undefined
      : fallbackProgress;''',
)
replace(
    "frontend/src/lib/components/JobRow.svelte",
    "  const percent = $derived(Math.round(Math.max(0, Math.min(1, job.progress ?? 0)) * 100));",
    "  const percent = $derived(job.progress === undefined ? undefined : Math.round(Math.max(0, Math.min(1, job.progress)) * 100));",
)
replace(
    "frontend/src/lib/components/JobRow.svelte",
    '''  <div class={`job-progress ${visualStatus}`} aria-label={`${percent}% complete`}>
    <div style={`width: ${percent}%`}></div>
  </div>
  <div class="job-meta">
    <span>{percent}%</span>
    {#if detail}<span class="job-detail">{detail}</span>{/if}
  </div>''',
    '''  {#if percent !== undefined}
    <div class={`job-progress ${visualStatus}`} aria-label={`${percent}% complete`}>
      <div style={`width: ${percent}%`}></div>
    </div>
  {/if}
  <div class="job-meta">
    {#if percent !== undefined}<span>{percent}%</span>{/if}
    {#if detail}<span class="job-detail">{detail}</span>{/if}
  </div>''',
)
replace(
    "docs/openapi.yaml",
    '''        progress_failed:
          type: integer
          format: int64
          minimum: 0
        created_at:''',
    '''        progress_failed:
          type: integer
          format: int64
          minimum: 0
        progress:
          type: number
          format: double
          minimum: 0
          maximum: 1
          description: Optional overall operation progress fraction. Uploads omit this while receiving when the request length is unknown.
        created_at:''',
)

# Server recovery test fake now distinguishes upload-visible recovery from hidden reservations.
replace(
    "internal/serve/background_operation_recovery_test.go",
    "type fakeBackgroundOperationReservationRecovery struct {\n\tkinds []string\n\terr   error\n}",
    "type fakeBackgroundOperationReservationRecovery struct {\n\thiddenKinds []string\n\tallKinds    []string\n\terr         error\n}",
)
replace(
    "internal/serve/background_operation_recovery_test.go",
    '''func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
\tf.kinds = append(f.kinds, kind)
\tif f.err != nil {
\t\treturn 0, f.err
\t}
\treturn 1, nil
}''',
    '''func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
\tf.hiddenKinds = append(f.hiddenKinds, kind)
\tif f.err != nil {
\t\treturn 0, f.err
\t}
\treturn 1, nil
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedBackgroundOperations(kind string) (int64, error) {
\tf.allKinds = append(f.allKinds, kind)
\tif f.err != nil {
\t\treturn 0, f.err
\t}
\treturn 1, nil
}''',
)
replace(
    "internal/serve/background_operation_recovery_test.go",
    '''\twant := []string{backgroundUploadImportOperationKind, core.BackgroundTagMutationOperationKind}
\tif len(recovery.kinds) != len(want) || recovery.kinds[0] != want[0] || recovery.kinds[1] != want[1] {
\t\tt.Fatalf("recovery kinds = %q, want %q", recovery.kinds, want)
\t}''',
    '''\tif len(recovery.allKinds) != 1 || recovery.allKinds[0] != backgroundUploadImportOperationKind {
\t\tt.Fatalf("visible-capable recovery kinds = %q, want upload", recovery.allKinds)
\t}
\tif len(recovery.hiddenKinds) != 1 || recovery.hiddenKinds[0] != core.BackgroundTagMutationOperationKind {
\t\tt.Fatalf("hidden recovery kinds = %q, want tag mutation", recovery.hiddenKinds)
\t}''',
)

# Durable upload fake supports pre-body checkpoint + visibility.
replace(
    "internal/serve/uploads_durable_test.go",
    '''\tterminalOnAttach   core.BackgroundWorkStatus
\tcancelCalls        int
}''',
    '''\tterminalOnAttach    core.BackgroundWorkStatus
\tcancelCalls         int
\tvisibleCalls        int
\tcheckpointCalls     int
\treceivingCheckpoint backgroundUploadCheckpoint
}''',
)
marker = "func (s *durableUploadTestStore) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, error) {"
insert = '''func (s *durableUploadTestStore) SetBackgroundOperationCheckpoint(operationID string, checkpoint any) error {
\tif operationID != s.operation.ID {
\t\treturn errors.New("unexpected operation")
\t}
\ts.checkpointCalls++
\ts.receivingCheckpoint = checkpoint.(backgroundUploadCheckpoint)
\treturn nil
}

func (s *durableUploadTestStore) SetBackgroundOperationVisible(operationID string, visible bool) error {
\tif operationID != s.operation.ID {
\t\treturn errors.New("unexpected operation")
\t}
\ts.visibleCalls++
\ts.state.Visible = visible
\treturn nil
}

'''
replace("internal/serve/uploads_durable_test.go", marker, insert + marker)
replace(
    "internal/serve/uploads_durable_test.go",
    '''\tif store.createdRequest.Kind != "upload_import" || store.createdRequest.Visible || store.createdRequest.ProgressTotal != 1 {
\t\tt.Fatalf("unexpected operation request: %+v", store.createdRequest)
\t}''',
    '''\tif store.createdRequest.Kind != "upload_import" || store.createdRequest.Visible || store.createdRequest.ProgressTotal != 1 {
\t\tt.Fatalf("unexpected operation request: %+v", store.createdRequest)
\t}
\tif store.visibleCalls != 1 || store.checkpointCalls < 1 || store.receivingCheckpoint.Phase != backgroundUploadPhaseReceiving {
\t\tt.Fatalf("receiving publication = visible %d checkpoints %d checkpoint %+v", store.visibleCalls, store.checkpointCalls, store.receivingCheckpoint)
\t}''',
)

# Overall DTO regressions.
replace(
    "internal/serve/background_operations_upload_progress_test.go",
    '''\tif dto.ProgressTotal != 1000 || dto.ProgressCompleted != 420 || dto.ProgressCompletedPrefix != 417 {
\t\tt.Fatalf("projected upload progress = %+v", dto)
\t}
}''',
    '''\tif dto.ProgressTotal != 1000 || dto.ProgressCompleted != 420 || dto.ProgressCompletedPrefix != 417 {
\t\tt.Fatalf("projected upload progress = %+v", dto)
\t}
\tif dto.Progress == nil || *dto.Progress < 0.709999 || *dto.Progress > 0.710001 {
\t\tt.Fatalf("overall upload progress = %v, want 0.71", dto.Progress)
\t}
}

func TestBackgroundOperationDTOProjectsReceivingUploadProgress(t *testing.T) {
\toperation := core.BackgroundOperationState{ID: "operation-upload-receiving", Kind: backgroundUploadImportOperationKind, Visible: true, Status: core.BackgroundWorkPending, ProgressTotal: 1, CreatedAt: time.Now().UTC()}
\treader := &fakeBackgroundOperationReader{checkpoints: map[string]backgroundUploadCheckpoint{
\t\toperation.ID: backgroundUploadReceivingCheckpoint(1000, 400),
\t}}
\tdto := (&Server{backgroundOperations: reader}).backgroundOperationDTO(operation)
\tif dto.Progress == nil || *dto.Progress < 0.199999 || *dto.Progress > 0.200001 {
\t\tt.Fatalf("receiving progress = %v, want 0.2", dto.Progress)
\t}
\tif dto.ProgressTotal != 1 || dto.ProgressCompleted != 0 {
\t\tt.Fatalf("receiving should preserve raw durable counters: %+v", dto)
\t}
}

func TestBackgroundOperationDTOLeavesUnknownLengthReceivingIndeterminate(t *testing.T) {
\toperation := core.BackgroundOperationState{ID: "operation-upload-receiving", Kind: backgroundUploadImportOperationKind, Visible: true, Status: core.BackgroundWorkPending, ProgressTotal: 1, CreatedAt: time.Now().UTC()}
\treader := &fakeBackgroundOperationReader{checkpoints: map[string]backgroundUploadCheckpoint{
\t\toperation.ID: backgroundUploadReceivingCheckpoint(0, 400),
\t}}
\tdto := (&Server{backgroundOperations: reader}).backgroundOperationDTO(operation)
\tif dto.Progress != nil {
\t\tt.Fatalf("unknown-length receiving progress = %v, want nil", *dto.Progress)
\t}
}''',
)

# Database recovery + attachment regressions.
p = Path("internal/database/durable_work_recovery_test.go")
p.write_text(p.read_text() + '''

func TestCancelUnattachedBackgroundOperationsIncludesVisibleReceivingWork(t *testing.T) {
\tstore, db := newDurableWorkTestDB(t)
\tstore.DB = db
\tif _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "visible-receiving", Kind: "upload_import", Visible: true}); err != nil {
\t\tt.Fatal(err)
\t}
\tif _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "other", Kind: "thumbnail", Visible: true}); err != nil {
\t\tt.Fatal(err)
\t}
\tcount, err := store.CancelUnattachedBackgroundOperations("upload_import")
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tif count != 1 {
\t\tt.Fatalf("canceled %d operations, want 1", count)
\t}
\tvar status string
\tif err := db.QueryRow(`SELECT status FROM background_operations WHERE id = 'visible-receiving'`).Scan(&status); err != nil {
\t\tt.Fatal(err)
\t}
\tif status != "canceled" {
\t\tt.Fatalf("visible receiving status = %q, want canceled", status)
\t}
}
''')
p = Path("internal/database/durable_work_attachment_test.go")
p.write_text(p.read_text() + '''

func TestAttachBackgroundTaskAcceptsVisibleReceivingReservation(t *testing.T) {
\tstore, db := newDurableWorkTestDB(t)
\tstore.DB = db
\tif _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "visible-upload", Kind: "upload_import", Visible: true, ProgressTotal: 1}); err != nil {
\t\tt.Fatal(err)
\t}
\tif _, err := store.AttachBackgroundTaskAndRevealOperation("visible-upload", []byte(`{"phase":"staged"}`), NewBackgroundTask{ID: "upload-task", DedupeKey: "upload", Kind: "upload.import", SubjectKind: "operation", SubjectID: "visible-upload", ResourceClass: "upload", MaxAttempts: 5}); err != nil {
\t\tt.Fatalf("attach visible upload: %v", err)
\t}
\tvar count int
\tif err := db.QueryRow(`SELECT count(*) FROM background_tasks WHERE operation_id = 'visible-upload'`).Scan(&count); err != nil {
\t\tt.Fatal(err)
\t}
\tif count != 1 {
\t\tt.Fatalf("attached task count = %d, want 1", count)
\t}
}
''')

# Frontend adapter regressions.
p = Path("frontend/src/lib/api/operations.progress.test.ts")
text = p.read_text()
text = text.replace("      progress_failed: 1,\n      created_at", "      progress_failed: 1,\n      progress: 0.71,\n      created_at", 1)
text = text.replace("    expect(job.progress).toBe(0.8);", "    expect(job.progress).toBe(0.71);", 1)
text = text.replace(
    "  });\n});",
    '''  });

  it('leaves receiving uploads indeterminate when the server has no overall fraction', () => {
    const operation: BackgroundOperation = {
      id: 'upload-receiving',
      kind: 'upload_import',
      status: 'pending',
      progress_total: 1,
      progress_completed: 0,
      progress_failed: 0,
      created_at: '2026-09-11T00:00:00Z'
    };
    expect(backgroundOperationAsJob(operation).progress).toBeUndefined();
  });
});''',
    1,
)
p.write_text(text)

# Request-body progress wrapper.
Path("internal/serve/upload_receiving_progress.go").write_text('''package serve

import (
\t"errors"
\t"io"
\t"time"

\tcore "gooru.local/gooru"
)

const (
\tuploadReceivingProgressSteps         int64 = 50
\tuploadReceivingUnknownByteStep       int64 = 4 << 20
\tuploadReceivingProgressMaxInterval         = 250 * time.Millisecond
)

var errUploadReceivingCanceled = errors.New("upload operation was canceled while receiving request body")

type uploadReceivingProgressStore interface {
\tSetBackgroundOperationCheckpoint(string, any) error
\tGetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
}

type uploadReceivingProgressReadCloser struct {
\tio.ReadCloser
\toperationID   string
\ttotal         int64
\treceived      int64
\tlastPersisted int64
\tlastAt        time.Time
\tstore         uploadReceivingProgressStore
}

func newUploadReceivingProgressReadCloser(body io.ReadCloser, operationID string, total int64, store uploadReceivingProgressStore) *uploadReceivingProgressReadCloser {
\tif total < 0 {
\t\ttotal = 0
\t}
\treturn &uploadReceivingProgressReadCloser{ReadCloser: body, operationID: operationID, total: total, lastAt: time.Now(), store: store}
}

func (r *uploadReceivingProgressReadCloser) Read(p []byte) (int, error) {
\tn, readErr := r.ReadCloser.Read(p)
\tif n > 0 {
\t\tr.received += int64(n)
\t}
\tforce := errors.Is(readErr, io.EOF)
\tif n > 0 || force {
\t\tif err := r.persist(force); err != nil {
\t\t\tif n > 0 {
\t\t\t\treturn n, err
\t\t\t}
\t\t\treturn 0, err
\t\t}
\t}
\treturn n, readErr
}

func (r *uploadReceivingProgressReadCloser) persist(force bool) error {
\tstep := uploadReceivingUnknownByteStep
\tif r.total > 0 {
\t\tstep = r.total / uploadReceivingProgressSteps
\t\tif step < 1 {
\t\t\tstep = 1
\t\t}
\t}
\tif !force && r.received-r.lastPersisted < step && time.Since(r.lastAt) < uploadReceivingProgressMaxInterval {
\t\treturn nil
\t}
\tcheckpoint := backgroundUploadReceivingCheckpoint(r.total, r.received)
\tif err := r.store.SetBackgroundOperationCheckpoint(r.operationID, checkpoint); err != nil {
\t\tstate, found, stateErr := r.store.GetBackgroundOperation(r.operationID)
\t\tif stateErr == nil && found && state.Status == core.BackgroundWorkCanceled {
\t\t\treturn errUploadReceivingCanceled
\t\t}
\t\t// Progress publication is observability, not the upload commit boundary.
\t\t// A transient checkpoint failure must not corrupt an otherwise valid body;
\t\t// later reads retry and the staged checkpoint replaces this state atomically.
\t\treturn nil
\t}
\tr.lastPersisted = r.received
\tr.lastAt = time.Now()
\treturn nil
}
''')
Path("internal/serve/upload_receiving_progress_test.go").write_text('''package serve

import (
\t"bytes"
\t"io"
\t"testing"

\tcore "gooru.local/gooru"
)

type receivingProgressTestStore struct {
\tcheckpoint backgroundUploadCheckpoint
\tstate      core.BackgroundOperationState
}

func (s *receivingProgressTestStore) SetBackgroundOperationCheckpoint(_ string, checkpoint any) error {
\ts.checkpoint = checkpoint.(backgroundUploadCheckpoint)
\treturn nil
}

func (s *receivingProgressTestStore) GetBackgroundOperation(_ string) (core.BackgroundOperationState, bool, error) {
\treturn s.state, true, nil
}

func TestUploadReceivingProgressPersistsKnownLengthBytes(t *testing.T) {
\tpayload := bytes.Repeat([]byte("x"), 1000)
\tstore := &receivingProgressTestStore{state: core.BackgroundOperationState{Status: core.BackgroundWorkPending}}
\tbody := newUploadReceivingProgressReadCloser(io.NopCloser(bytes.NewReader(payload)), "operation", int64(len(payload)), store)
\tif _, err := io.Copy(io.Discard, body); err != nil {
\t\tt.Fatal(err)
\t}
\tif store.checkpoint.Phase != backgroundUploadPhaseReceiving || store.checkpoint.TransportBytesTotal != 1000 || store.checkpoint.TransportBytesReceived != 1000 {
\t\tt.Fatalf("receiving checkpoint = %+v", store.checkpoint)
\t}
}

func TestUploadReceivingProgressUnknownLengthKeepsTotalUnknown(t *testing.T) {
\tpayload := bytes.Repeat([]byte("x"), 128)
\tstore := &receivingProgressTestStore{state: core.BackgroundOperationState{Status: core.BackgroundWorkPending}}
\tbody := newUploadReceivingProgressReadCloser(io.NopCloser(bytes.NewReader(payload)), "operation", -1, store)
\tif _, err := io.Copy(io.Discard, body); err != nil {
\t\tt.Fatal(err)
\t}
\tif store.checkpoint.TransportBytesTotal != 0 || store.checkpoint.TransportBytesReceived != int64(len(payload)) {
\t\tt.Fatalf("unknown-length checkpoint = %+v", store.checkpoint)
\t}
}
''')
