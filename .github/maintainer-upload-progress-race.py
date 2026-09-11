from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"missing snippet in {path}: {old[:100]!r}")
    p.write_text(text.replace(old, new, 1))

replace(
    "internal/serve/uploads_durable.go",
    '''// durableUploadOperationStore is the producer/read boundary required by HTTP
// uploads. The operation is created hidden before multipart staging, then the
// recovery checkpoint, child task, and visibility transition are committed
// atomically once staging succeeds.''',
    '''// durableUploadOperationStore is the producer/read boundary required by HTTP
// uploads. Admission starts hidden, then the receiving checkpoint is published
// before multipart staging so the same operation is visible throughout transport
// and durable import.''',
)
replace(
    "internal/serve/uploads_durable.go",
    '''\tif _, err := operations.AttachBackgroundTaskAndRevealOperation(operation.ID, backgroundUploadInitialCheckpoint(len(saved)), taskRequest); err != nil {
\t\tremoveSavedUploads(saved)
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to queue uploaded files", nil)
\t\treturn
\t}''',
    '''\tif _, err := operations.AttachBackgroundTaskAndRevealOperation(operation.ID, backgroundUploadInitialCheckpoint(len(saved)), taskRequest); err != nil {
\t\tremoveSavedUploads(saved)
\t\tif state, found, stateErr := operations.GetBackgroundOperation(operation.ID); stateErr == nil && found && state.Status == core.BackgroundWorkCanceled {
\t\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "upload was canceled", nil)
\t\t\treturn
\t\t}
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to queue uploaded files", nil)
\t\treturn
\t}''',
)

# Test fake can force attachment to lose a cancellation race.
replace(
    "internal/serve/uploads_durable_test.go",
    '''\tterminalOnAttach    core.BackgroundWorkStatus
\tcancelCalls         int''',
    '''\tterminalOnAttach    core.BackgroundWorkStatus
\tattachErr           error
\tcancelCalls         int''',
)
replace(
    "internal/serve/uploads_durable_test.go",
    '''\tif s.terminalOnAttach != "" {
\t\ts.state.Status = s.terminalOnAttach
\t}
\tif s.attached != nil {''',
    '''\tif s.terminalOnAttach != "" {
\t\ts.state.Status = s.terminalOnAttach
\t}
\tif s.attached != nil {''',
)
replace(
    "internal/serve/uploads_durable_test.go",
    '''\t\tclose(s.attached)
\t\ts.attached = nil
\t}
\treturn core.BackgroundTask{ID: "task-upload-test", OperationID: operationID, Kind: request.Kind, SubjectKind: request.SubjectKind, SubjectID: request.SubjectID, InputKey: request.InputKey}, nil
}''',
    '''\t\tclose(s.attached)
\t\ts.attached = nil
\t}
\tif s.attachErr != nil {
\t\treturn core.BackgroundTask{}, s.attachErr
\t}
\treturn core.BackgroundTask{ID: "task-upload-test", OperationID: operationID, Kind: request.Kind, SubjectKind: request.SubjectKind, SubjectID: request.SubjectID, InputKey: request.InputKey}, nil
}''',
)

# Blocking body proves visibility is established before the first multipart read can finish.
replace(
    "internal/serve/uploads_durable_test.go",
    '"reflect"\n\t"testing"',
    '"reflect"\n\t"sync"\n\t"testing"',
)
insert_marker = '''type uploadReadTracker struct {
\tread bool
}
'''
insert = '''type gatedUploadBody struct {
\tio.ReadCloser
\tentered chan struct{}
\trelease chan struct{}
\tonce    sync.Once
}

func (b *gatedUploadBody) Read(p []byte) (int, error) {
\tb.once.Do(func() {
\t\tclose(b.entered)
\t\t<-b.release
\t})
\treturn b.ReadCloser.Read(p)
}

'''
replace("internal/serve/uploads_durable_test.go", insert_marker, insert + insert_marker)
append_marker = '''func TestDurableUploadAsyncAttachesOneTaskAndReturnsOperationLocation(t *testing.T) {'''
new_test = '''func TestDurableUploadPublishesOperationWhileRequestBodyIsStillReceiving(t *testing.T) {
\ttargetDir := t.TempDir()
\tstore := newDurableUploadTestStore()
\tserver := newDurableUploadHandlerTestServer(t, targetDir, store)
\treq := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
\treq.Header.Set("Prefer", "respond-async")
\tbody := &gatedUploadBody{ReadCloser: req.Body, entered: make(chan struct{}), release: make(chan struct{})}
\treq.Body = body
\trec := httptest.NewRecorder()
\tdone := make(chan struct{})
\tgo func() {
\t\tserver.Handler().ServeHTTP(rec, req)
\t\tclose(done)
\t}()

\t<-body.entered
\tif !store.state.Visible || store.visibleCalls != 1 {
\t\tt.Fatalf("operation was not visible before body read completed: state=%+v calls=%d", store.state, store.visibleCalls)
\t}
\tif store.receivingCheckpoint.Phase != backgroundUploadPhaseReceiving {
\t\tt.Fatalf("checkpoint phase while body blocked = %q, want receiving", store.receivingCheckpoint.Phase)
\t}
\tclose(body.release)
\t<-done
\tif rec.Code != http.StatusAccepted {
\t\tt.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
\t}
}

'''
replace("internal/serve/uploads_durable_test.go", append_marker, new_test + append_marker)

# Cancellation between staging and attachment should be surfaced as cancellation,
# not an internal queue error, and staged bytes must be removed.
p = Path("internal/serve/uploads_durable_test.go")
text = p.read_text()
text += '''

func TestDurableUploadCancellationWinningAttachmentRaceReturnsCanceled(t *testing.T) {
\ttargetDir := t.TempDir()
\tstore := newDurableUploadTestStore()
\tstore.terminalOnAttach = core.BackgroundWorkCanceled
\tstore.attachErr = errors.New("operation canceled before attach")
\tserver := newDurableUploadHandlerTestServer(t, targetDir, store)
\treq := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
\treq.Header.Set("Prefer", "respond-async")
\trec := httptest.NewRecorder()

\tserver.Handler().ServeHTTP(rec, req)

\tassertAPIError(t, rec, http.StatusRequestTimeout, "request_canceled")
\tif _, err := os.Stat(filepath.Join(targetDir, "photo.jpg")); !os.IsNotExist(err) {
\t\tt.Fatalf("staged file remained after canceled attachment race: %v", err)
\t}
}
'''
p.write_text(text)
