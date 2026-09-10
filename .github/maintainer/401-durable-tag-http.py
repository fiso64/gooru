#!/usr/bin/env python3
from pathlib import Path

def write(path: str, content: str) -> None:
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8")

def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one replacement target, found {count}")
    p.write_text(text.replace(old, new, 1), encoding="utf-8")

# Core request needs to distinguish an empty resolved file-selection from a query.
replace_once(
    "gooru/background_tag_mutation.go",
    '''\tbackgroundTagMutationTargetFileID      = "file_id"
\tbackgroundTagMutationTargetContentHash = "content_hash"
\tbackgroundTagMutationInputVersion      = 1
''',
    '''\tBackgroundTagMutationTargetFileID      = "file_id"
\tBackgroundTagMutationTargetContentHash = "content_hash"
\tbackgroundTagMutationInputVersion      = 1
''',
)
replace_once(
    "gooru/background_tag_mutation.go",
    '''\tFileIDs        []string
\tQuery          string
\tExcludedHashes []string
\tMaxPending     int
''',
    '''\tFileIDs        []string
\tQuery          string
\tFileIDSelector bool
\tExcludedHashes []string
\tMaxPending     int
''',
)
replace_once(
    "gooru/background_tag_mutation.go",
    '''\tif (len(request.FileIDs) == 0) == (request.Query == "") {
\t\treturn BackgroundOperation{}, errors.New("background tag mutation requires exactly one target selector")
\t}
\tif request.Query == "" && len(request.ExcludedHashes) != 0 {
''',
    '''\tif request.FileIDSelector {
\t\tif request.Query != "" {
\t\t\treturn BackgroundOperation{}, errors.New("background tag mutation file-id selector cannot use a query")
\t\t}
\t} else if request.Query == "" || len(request.FileIDs) != 0 {
\t\treturn BackgroundOperation{}, errors.New("background tag mutation requires exactly one target selector")
\t}
\tif request.FileIDSelector && len(request.ExcludedHashes) != 0 {
''',
)
replace_once(
    "gooru/background_tag_mutation.go",
    '''\ttargetKind := backgroundTagMutationTargetFileID
\tvar targetQuery string
\tvar targetArgs []interface{}
\tif request.Query != "" {
\t\ttargetKind = backgroundTagMutationTargetContentHash
''',
    '''\ttargetKind := BackgroundTagMutationTargetFileID
\tvar targetQuery string
\tvar targetArgs []interface{}
\tif !request.FileIDSelector {
\t\ttargetKind = BackgroundTagMutationTargetContentHash
''',
)
replace_once(
    "gooru/background_tag_mutation.go",
    'if state.TargetKind != backgroundTagMutationTargetContentHash {',
    'if state.TargetKind != BackgroundTagMutationTargetContentHash {',
)
replace_once(
    "gooru/background_tag_mutation.go",
    'if state.TargetKind != backgroundTagMutationTargetFileID {',
    'if state.TargetKind != BackgroundTagMutationTargetFileID {',
)
replace_once(
    "gooru/background_tag_mutation.go",
    '''\tif len(paths) != state.MatchedFiles {
\t\treturn fmt.Errorf("background tag mutation target count changed: expected %d paths, got %d", state.MatchedFiles, len(paths))
\t}
\tkind, err := backgroundTagMutationKind(state.Mutation, state.Tags)
''',
    '''\tif len(paths) != state.MatchedFiles {
\t\treturn fmt.Errorf("background tag mutation target count changed: expected %d paths, got %d", state.MatchedFiles, len(paths))
\t}
\tif len(paths) == 0 {
\t\ttx, err := c.store.Begin()
\t\tif err != nil {
\t\t\treturn err
\t\t}
\t\tdefer tx.Rollback()
\t\tif err := c.persistBackgroundTagMutationResultTx(tx, operationID, types.TagOperationResult{}); err != nil {
\t\t\treturn err
\t\t}
\t\treturn tx.Commit()
\t}
\tkind, err := backgroundTagMutationKind(state.Mutation, state.Tags)
''',
)
replace_once(
    "gooru/background_tag_mutation_test.go",
    '''\t\tFileIDs:    []string{publicID},
\t\tMaxPending: 8,
''',
    '''\t\tFileIDs:        []string{publicID},
\t\tFileIDSelector: true,
\t\tMaxPending:     8,
''',
)

write("internal/serve/background_tag_mutations.go", r'''package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	core "gooru.local/gooru"
)

const defaultDurableTagMutationPendingLimit = 64

type durableTagMutationLibrary interface {
	createBackgroundTagMutation(context.Context, TagOperation, TagMutationSelector, TagMutationRequest, int) (core.BackgroundOperation, error)
	executeBackgroundTagMutation(context.Context, core.BackgroundTask) error
}

func (l *GooruLibrary) createBackgroundTagMutation(
	ctx context.Context,
	operation TagOperation,
	selector TagMutationSelector,
	request TagMutationRequest,
	maxPending int,
) (core.BackgroundOperation, error) {
	if err := ctx.Err(); err != nil {
		return core.BackgroundOperation{}, err
	}
	excludedHashes := make([]string, 0, len(request.ExcludeFileIDs))
	for _, encoded := range request.ExcludeFileIDs {
		file, err := l.GetFileByPublicID(ctx, encoded)
		if err != nil {
			return core.BackgroundOperation{}, err
		}
		excludedHashes = append(excludedHashes, file.Hash)
	}
	return l.client.CreateBackgroundTagMutation(core.BackgroundTagMutationRequest{
		Mutation:       string(operation),
		Selector:       selector,
		Tags:           request.Tags,
		FileIDs:        request.FileIDs,
		Query:          request.Query,
		FileIDSelector: request.Query == "",
		ExcludedHashes: excludedHashes,
		MaxPending:     maxPending,
	})
}

func (l *GooruLibrary) executeBackgroundTagMutation(ctx context.Context, task core.BackgroundTask) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if task.Kind != core.BackgroundTagMutationTaskKind ||
		task.SubjectKind != "operation" ||
		task.SubjectID == "" ||
		task.SubjectID != task.OperationID ||
		task.InputKey != "v1" {
		return errors.New("tag mutation background task has invalid identity")
	}
	state, found, err := l.client.GetBackgroundTagMutation(task.OperationID)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("tag mutation background state is missing")
	}
	if state.ResultReady {
		return nil
	}
	switch state.TargetKind {
	case core.BackgroundTagMutationTargetContentHash:
		return l.client.ExecuteBackgroundTagMutationQuery(task.OperationID)
	case core.BackgroundTagMutationTargetFileID:
		targets, err := l.client.ListBackgroundTagMutationTargets(task.OperationID)
		if err != nil {
			return err
		}
		paths := make([]string, 0, len(targets))
		for _, target := range targets {
			if err := ctx.Err(); err != nil {
				return err
			}
			file, err := l.GetFileByPublicID(ctx, target)
			if err != nil {
				return err
			}
			paths = append(paths, file.Path)
		}
		return l.client.ExecuteBackgroundTagMutationPaths(task.OperationID, paths)
	default:
		return fmt.Errorf("tag mutation background state has invalid target kind %q", state.TargetKind)
	}
}

func (l *GooruLibrary) backgroundTagMutationResponse(operationID string) (TagMutationResponse, bool, error) {
	state, found, err := l.client.GetBackgroundTagMutation(operationID)
	if err != nil || !found {
		return TagMutationResponse{}, found, err
	}
	if !state.ResultReady {
		return TagMutationResponse{}, false, nil
	}
	var selector TagMutationSelector
	if err := json.Unmarshal(state.SelectorJSON, &selector); err != nil {
		return TagMutationResponse{}, false, fmt.Errorf("decode background tag mutation selector: %w", err)
	}
	return TagMutationResponse{
		Operation:     TagOperation(state.Mutation),
		Selector:      selector,
		MatchedFiles:  state.MatchedFiles,
		AffectedCount: state.AffectedCount,
		Notifications: notificationDTOs(state.Notifications),
	}, true, nil
}

func (s *Server) backgroundTagMutationHandler(ctx context.Context, task core.BackgroundTask) error {
	mutator, ok := s.library.(durableTagMutationLibrary)
	if !ok || mutator == nil {
		return errors.New("durable tag mutation service is not configured")
	}
	return mutator.executeBackgroundTagMutation(ctx, task)
}

func waitForDurableTagMutation(ctx context.Context, operations backgroundOperationReader, operationID string) (TagMutationResponse, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, found, err := operations.GetBackgroundOperation(operationID)
		if err != nil {
			return TagMutationResponse{}, err
		}
		if !found {
			return TagMutationResponse{}, errors.New("tag mutation operation disappeared")
		}
		switch state.Status {
		case core.BackgroundWorkCompleted:
			var response TagMutationResponse
			found, err := operations.GetBackgroundOperationResult(operationID, &response)
			if err != nil {
				return TagMutationResponse{}, err
			}
			if !found {
				return TagMutationResponse{}, errors.New("completed tag mutation operation has no result")
			}
			return response, nil
		case core.BackgroundWorkFailed:
			if state.ErrorMessage != "" {
				return TagMutationResponse{}, errors.New(state.ErrorMessage)
			}
			return TagMutationResponse{}, errors.New("tag mutation operation failed")
		case core.BackgroundWorkCanceled:
			return TagMutationResponse{}, context.Canceled
		}
		select {
		case <-ctx.Done():
			return TagMutationResponse{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func writeDurableTagMutationAdmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrBackgroundOperationPendingLimit):
		writeError(w, http.StatusServiceUnavailable, "job_queue_full", "job queue is full", nil)
	case errors.Is(err, core.ErrInvalidQuery):
		writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to accept tag mutation", nil)
	}
}
''')

# background_tag_mutations.go uses HTTP error helpers.
replace_once(
    "internal/serve/background_tag_mutations.go",
    '''\t"fmt"
\t"time"

\tcore "gooru.local/gooru"
''',
    '''\t"fmt"
\t"net/http"
\t"time"

\tcore "gooru.local/gooru"
''',
)

# Replace JobManager submission with durable operation admission/polling plus a
# synchronous-only fallback for non-production test/library implementations.
old_handler_prefix = '''\tmutator, ok := s.library.(TagMutationLibrary)
\tif s.library == nil || !ok {
\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "tag mutation service is not configured", nil)
\t\treturn
\t}
'''
new_handler_prefix = '''\tlegacyMutator, legacyOK := s.library.(TagMutationLibrary)
\tdurableMutator, durableOK := s.library.(durableTagMutationLibrary)
\tif s.library == nil || (!legacyOK && !durableOK) {
\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "tag mutation service is not configured", nil)
\t\treturn
\t}
'''
replace_once("internal/serve/tag_mutations.go", old_handler_prefix, new_handler_prefix)

old_submit = '''\tjob, err := s.jobs.Submit(r.Context(), "tag_mutation", PreferAsync(r), func(ctx context.Context) (interface{}, error) {
\t\tif request.SelectionID != "" && len(resolvedRequest.FileIDs) == 0 {
\t\t\treturn TagMutationResponse{Operation: operation, Selector: selector}, nil
\t\t}
\t\tresponse, err := mutator.MutateTags(ctx, operation, resolvedRequest)
\t\tif err != nil {
\t\t\treturn TagMutationResponse{}, err
\t\t}
\t\tresponse.Selector = selector
\t\treturn response, nil
\t})
\tif PreferAsync(r) && err == nil {
\t\twriteJSON(w, http.StatusAccepted, job)
\t\treturn
\t}
\tif err != nil {
\t\twriteJobSubmitError(w, err, "failed to mutate tags")
\t\treturn
\t}
\tresponse, ok := job.Result.(TagMutationResponse)
\tif !ok {
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to mutate tags", nil)
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, response)
'''
new_submit = '''\tif !durableOK {
\t\tif PreferAsync(r) {
\t\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "durable tag mutation service is not configured", nil)
\t\t\treturn
\t\t}
\t\tif request.SelectionID != "" && len(resolvedRequest.FileIDs) == 0 {
\t\t\twriteJSON(w, http.StatusOK, TagMutationResponse{Operation: operation, Selector: selector})
\t\t\treturn
\t\t}
\t\tresponse, err := legacyMutator.MutateTags(r.Context(), operation, resolvedRequest)
\t\tif err != nil {
\t\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to mutate tags", nil)
\t\t\treturn
\t\t}
\t\tresponse.Selector = selector
\t\twriteJSON(w, http.StatusOK, response)
\t\treturn
\t}
\tif s.backgroundOperations == nil {
\t\twriteError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
\t\treturn
\t}
\toperationState, err := durableMutator.createBackgroundTagMutation(
\t\tr.Context(),
\t\toperation,
\t\tselector,
\t\tresolvedRequest,
\t\tdefaultDurableTagMutationPendingLimit,
\t)
\tif err != nil {
\t\twriteDurableTagMutationAdmissionError(w, err)
\t\treturn
\t}
\tstate, found, err := s.backgroundOperations.GetBackgroundOperation(operationState.ID)
\tif err != nil || !found {
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to load tag mutation operation", nil)
\t\treturn
\t}
\tif PreferAsync(r) {
\t\tw.Header().Set("Location", "/api/v1/operations/"+operationState.ID)
\t\twriteJSON(w, http.StatusAccepted, backgroundOperationDTO(state))
\t\treturn
\t}
\tresponse, err := waitForDurableTagMutation(r.Context(), s.backgroundOperations, operationState.ID)
\tif err != nil {
\t\tif errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
\t\t\t_, _ = s.cancelBackgroundOperation(operationState.ID)
\t\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
\t\t\treturn
\t\t}
\t\twriteError(w, http.StatusInternalServerError, "internal_error", "failed to mutate tags", nil)
\t\treturn
\t}
\twriteJSON(w, http.StatusOK, response)
'''
replace_once("internal/serve/tag_mutations.go", old_submit, new_submit)

# Dedicated exact tag results are served through the generic durable operation API.
replace_once(
    "internal/serve/background_operations.go",
    '''func (l *GooruLibrary) GetBackgroundOperationResult(operationID string, destination any) (bool, error) {
\treturn l.client.GetBackgroundOperationResult(operationID, destination)
}
''',
    '''func (l *GooruLibrary) GetBackgroundOperationResult(operationID string, destination any) (bool, error) {
\toperation, found, err := l.client.GetBackgroundOperation(operationID)
\tif err != nil {
\t\treturn false, err
\t}
\tif found && operation.Kind == core.BackgroundTagMutationOperationKind {
\t\tif operation.Status != core.BackgroundWorkCompleted {
\t\t\treturn false, nil
\t\t}
\t\tresponse, found, err := l.backgroundTagMutationResponse(operationID)
\t\tif err != nil || !found {
\t\t\treturn found, err
\t\t}
\t\tencoded, err := json.Marshal(response)
\t\tif err != nil {
\t\t\treturn false, err
\t\t}
\t\tif err := json.Unmarshal(encoded, destination); err != nil {
\t\t\treturn false, err
\t\t}
\t\treturn true, nil
\t}
\treturn l.client.GetBackgroundOperationResult(operationID, destination)
}
''',
)

# Register the durable metadata worker.
replace_once(
    "internal/serve/background_thumbnails.go",
    '''\tuploadRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
\t\tResourceClass: backgroundUploadResourceClass,
\t\tWorkerID:      workerID + "-upload",
\t\tHandlers: map[string]core.BackgroundTaskHandler{
\t\t\tbackgroundUploadTaskKind: s.backgroundUploadHandler(client),
\t\t},
\t})
\tif err != nil {
\t\treturn nil, err
\t}
\treturn multiBackgroundRuntime{runtimes: []BackgroundRuntime{mediaRuntime, storageRuntime, uploadRuntime}}, nil
''',
    '''\tuploadRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
\t\tResourceClass: backgroundUploadResourceClass,
\t\tWorkerID:      workerID + "-upload",
\t\tHandlers: map[string]core.BackgroundTaskHandler{
\t\t\tbackgroundUploadTaskKind: s.backgroundUploadHandler(client),
\t\t},
\t})
\tif err != nil {
\t\treturn nil, err
\t}
\tmetadataRuntime, err := client.NewBackgroundRuntime(core.BackgroundWorkerConfig{
\t\tResourceClass: core.BackgroundTagMutationResourceClass,
\t\tWorkerID:      workerID + "-metadata",
\t\tHandlers: map[string]core.BackgroundTaskHandler{
\t\t\tcore.BackgroundTagMutationTaskKind: s.backgroundTagMutationHandler,
\t\t},
\t})
\tif err != nil {
\t\treturn nil, err
\t}
\treturn multiBackgroundRuntime{runtimes: []BackgroundRuntime{mediaRuntime, storageRuntime, uploadRuntime, metadataRuntime}}, nil
''',
)

# Recover hidden tag reservations as well as uploads.
write("internal/serve/background_operation_recovery.go", r'''package serve

import (
	"fmt"

	core "gooru.local/gooru"
)

const backgroundUploadImportOperationKind = "upload_import"

type backgroundOperationReservationRecovery interface {
	CancelUnattachedHiddenBackgroundOperations(string) (int64, error)
}

// recoverBackgroundOperationReservations releases producer admission reservations
// left behind before a durable task was attached. Startup invokes this before
// constructing workers and before the HTTP server begins accepting new producer
// requests, so only pre-existing hidden reservations are eligible.
func recoverBackgroundOperationReservations(recovery backgroundOperationReservationRecovery) error {
	if recovery == nil {
		return nil
	}
	for _, item := range []struct {
		kind string
		name string
	}{
		{kind: backgroundUploadImportOperationKind, name: "upload"},
		{kind: core.BackgroundTagMutationOperationKind, name: "tag mutation"},
	} {
		if _, err := recovery.CancelUnattachedHiddenBackgroundOperations(item.kind); err != nil {
			return fmt.Errorf("recover %s background operation reservations: %w", item.name, err)
		}
	}
	return nil
}

// CancelUnattachedHiddenBackgroundOperations exposes the core startup recovery
// primitive through the server library facade for producer composition.
func (l *GooruLibrary) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	return l.client.CancelUnattachedHiddenBackgroundOperations(kind)
}

var _ backgroundOperationReservationRecovery = (*core.Client)(nil)
''')

write("internal/serve/background_operation_recovery_test.go", r'''package serve

import (
	"errors"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type fakeBackgroundOperationReservationRecovery struct {
	kinds []string
	err   error
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	f.kinds = append(f.kinds, kind)
	if f.err != nil {
		return 0, f.err
	}
	return 1, nil
}

func TestRecoverBackgroundOperationReservationsTargetsDurableProducers(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{}
	if err := recoverBackgroundOperationReservations(recovery); err != nil {
		t.Fatal(err)
	}
	want := []string{backgroundUploadImportOperationKind, core.BackgroundTagMutationOperationKind}
	if len(recovery.kinds) != len(want) || recovery.kinds[0] != want[0] || recovery.kinds[1] != want[1] {
		t.Fatalf("recovery kinds = %q, want %q", recovery.kinds, want)
	}
}

func TestRecoverBackgroundOperationReservationsPropagatesFailure(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{err: errors.New("boom")}
	err := recoverBackgroundOperationReservations(recovery)
	if err == nil || !strings.Contains(err.Error(), "recover upload background operation reservations") {
		t.Fatalf("expected wrapped recovery error, got %v", err)
	}
}

func TestRecoverBackgroundOperationReservationsAllowsMissingRecovery(t *testing.T) {
	if err := recoverBackgroundOperationReservations(nil); err != nil {
		t.Fatalf("nil recovery returned %v", err)
	}
}
''')

# Replace legacy async/queue tests and run the real durable runtime for sync integration.
replace_once(
    "internal/serve/tag_mutations_test.go",
    '''\t"testing"

\t"gooru.local/types"
''',
    '''\t"testing"

\tcore "gooru.local/gooru"
\t"gooru.local/types"
''',
)

old_async_test = r'''func TestTagMutationAsyncReturnsJob(t *testing.T) {
	server := newTagMutationTestServer(t, &recordingMutationLibrary{file: types.FileInfo{ID: 6, Path: "/tmp/a.jpg"}})
	req := authedJSONRequest(http.MethodPut, "/api/v1/files/tags", `{"file_ids":["`+fallbackPublicFileID(6)+`"],"tags":["ready"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var job Job
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if job.ID == "" || job.Type != "tag_mutation" {
		t.Fatalf("unexpected async job: %+v", job)
	}
}

func TestTagMutationQueueFullReturnsStableJSONError(t *testing.T) {
	server := newTagMutationTestServer(t, &recordingMutationLibrary{file: types.FileInfo{ID: 7, Path: "/tmp/a.jpg"}})
	release := saturateJobQueue(t, server)
	defer release()
	req := authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+fallbackPublicFileID(7)+`"],"tags":["reviewed"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusServiceUnavailable, "job_queue_full")
}
'''
new_async_test = r'''func TestTagMutationAsyncReturnsDurableOperation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()
	page := listTestFiles(t, server, "kind:image", 1)

	req := authedJSONRequest(http.MethodPut, "/api/v1/files/tags", `{"file_ids":["`+page.Files[0].ID+`"],"tags":["ready"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var operation BackgroundOperationDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &operation); err != nil {
		t.Fatalf("decode operation: %v", err)
	}
	if operation.ID == "" || operation.Kind != core.BackgroundTagMutationOperationKind || operation.Status != core.BackgroundWorkPending {
		t.Fatalf("unexpected async operation: %+v", operation)
	}
	if got, want := rec.Header().Get("Location"), "/api/v1/operations/"+operation.ID; got != want {
		t.Fatalf("Location = %q, want %q", got, want)
	}
}

func TestTagMutationQueueFullReturnsStableJSONError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()
	page := listTestFiles(t, server, "kind:image", 1)

	for i := 0; i < defaultDurableTagMutationPendingLimit; i++ {
		if _, err := client.CreateBackgroundOperationWithPendingLimit(core.BackgroundOperationRequest{
			Kind:          core.BackgroundTagMutationOperationKind,
			Visible:       true,
			ProgressTotal: 1,
		}, defaultDurableTagMutationPendingLimit+1); err != nil {
			t.Fatalf("seed pending operation %d: %v", i, err)
		}
	}
	req := authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+page.Files[0].ID+`"],"tags":["reviewed"]}`)
	req.Header.Set("Prefer", "respond-async")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusServiceUnavailable, "job_queue_full")
}
'''
replace_once("internal/serve/tag_mutations_test.go", old_async_test, new_async_test)

old_integration_head = '''func TestTagMutationIntegrationUpdatesFileTags(t *testing.T) {
\tserver, cleanup := newTestBrowseServer(t)
\tdefer cleanup()

'''
new_integration_head = '''func TestTagMutationIntegrationUpdatesFileTags(t *testing.T) {
\tdir := t.TempDir()
\tdbPath := filepath.Join(dir, "gooru.db")
\tserver, client := newTestBrowseServerAt(t, dir, dbPath)
\tdefer client.Close()
\truntime, err := server.NewBackgroundRuntime(client, "tag-mutation-test")
\tif err != nil {
\t\tt.Fatalf("create background runtime: %v", err)
\t}
\tctx, cancel := context.WithCancel(context.Background())
\tdone := make(chan error, 1)
\tgo func() { done <- runtime.Run(ctx) }()
\tdefer func() {
\t\tcancel()
\t\tif err := <-done; err != nil && !errors.Is(err, context.Canceled) {
\t\t\tt.Fatalf("stop background runtime: %v", err)
\t\t}
\t}()

'''
replace_once("internal/serve/tag_mutations_test.go", old_integration_head, new_integration_head)

# The test now uses errors.
replace_once(
    "internal/serve/tag_mutations_test.go",
    '''\t"context"
\t"encoding/json"
''',
    '''\t"context"
\t"encoding/json"
\t"errors"
''',
)

# OpenAPI tag mutation async responses now return durable operations.
openapi = Path("docs/openapi.yaml")
text = openapi.read_text(encoding="utf-8")
start = text.index("  /files/tags:\n")
end = text.index("  /ui-state:\n", start)
section = text[start:end]
if section.count('$ref: "#/components/responses/AsyncJob"') != 3:
    raise SystemExit("docs/openapi.yaml: expected three tag mutation AsyncJob responses")
section = section.replace('$ref: "#/components/responses/AsyncJob"', '$ref: "#/components/responses/AsyncOperation"')
openapi.write_text(text[:start] + section + text[end:], encoding="utf-8")
