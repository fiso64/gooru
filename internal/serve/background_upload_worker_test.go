package serve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type recordingUploadImporter struct {
	calls     int
	response  UploadImportResponse
	err       error
	state     backgroundUploadImportState
	activated int
	before    func()
}

func (i *recordingUploadImporter) importUploadedFiles(_ context.Context, _ []StagedUpload, _ []string, state backgroundUploadImportState, activated []activatedSavedReplacement) (UploadImportResponse, error) {
	i.calls++
	i.state = state
	i.activated = len(activated)
	if i.before != nil {
		i.before()
	}
	return i.response, i.err
}

type recordingUploadWorkerStore struct {
	checkpoint               backgroundUploadCheckpoint
	found                    bool
	result                   UploadImportResponse
	events                   []string
	operationStatus          core.BackgroundWorkStatus
	progressTotal            int64
	checkpointErr            error
	resultErr                error
	operationCheckpointReads int
}

func (s *recordingUploadWorkerStore) GetBackgroundOperation(operationID string) (core.BackgroundOperationState, bool, error) {
	return core.BackgroundOperationState{ID: operationID, Kind: backgroundUploadImportOperationKind, Status: s.operationStatus, ProgressTotal: s.progressTotal}, true, nil
}

func (s *recordingUploadWorkerStore) GetBackgroundOperationCheckpoint(_ string, destination any) (bool, error) {
	s.operationCheckpointReads++
	if !s.found {
		return false, nil
	}
	checkpoint := destination.(*backgroundUploadCheckpoint)
	*checkpoint = s.checkpoint
	return true, nil
}

func (s *recordingUploadWorkerStore) SetBackgroundOperationCheckpoint(_ string, checkpoint any) error {
	if s.checkpointErr != nil {
		return s.checkpointErr
	}
	s.checkpoint = checkpoint.(backgroundUploadCheckpoint)
	s.found = true
	s.events = append(s.events, "checkpoint:"+s.checkpoint.Phase)
	return nil
}

func (s *recordingUploadWorkerStore) SetBackgroundOperationResult(_ string, result any) error {
	if s.resultErr != nil {
		return s.resultErr
	}
	s.result = result.(UploadImportResponse)
	s.events = append(s.events, "result")
	return nil
}

type recordingTaskUploadWorkerStore struct {
	*recordingUploadWorkerStore
	taskCheckpoint      backgroundUploadCheckpoint
	taskFound           bool
	taskResult          UploadImportResponse
	taskCheckpointReads int
	taskCheckpointErr   error
	taskResultErr       error
}

func (s *recordingTaskUploadWorkerStore) GetBackgroundTaskCheckpoint(_ string, destination any) (bool, error) {
	s.taskCheckpointReads++
	if s.taskCheckpointErr != nil {
		return false, s.taskCheckpointErr
	}
	if !s.taskFound {
		return false, nil
	}
	checkpoint := destination.(*backgroundUploadCheckpoint)
	*checkpoint = s.taskCheckpoint
	return true, nil
}

func (s *recordingTaskUploadWorkerStore) SetBackgroundTaskCheckpoint(_ string, checkpoint any) error {
	if s.taskCheckpointErr != nil {
		return s.taskCheckpointErr
	}
	s.taskCheckpoint = checkpoint.(backgroundUploadCheckpoint)
	s.taskFound = true
	s.events = append(s.events, "task-checkpoint:"+s.taskCheckpoint.Phase)
	return nil
}

func (s *recordingTaskUploadWorkerStore) SetBackgroundTaskResult(_ string, result any) error {
	if s.taskResultErr != nil {
		return s.taskResultErr
	}
	s.taskResult = result.(UploadImportResponse)
	s.events = append(s.events, "task-result")
	return nil
}

func backgroundUploadWorkerTestTask(t *testing.T, operationID string) core.BackgroundTask {
	t.Helper()
	return backgroundUploadWorkerTaskForFiles(t, operationID, []savedUpload{{
		name:     "already-rejected.jpg",
		size:     12,
		targetID: "default",
		status:   "error",
		error:    "rejected",
	}})
}

func backgroundUploadWorkerTaskForFiles(t *testing.T, operationID string, files []savedUpload) core.BackgroundTask {
	t.Helper()
	request, err := backgroundUploadTaskRequest(operationID, files, []string{"artist:test"})
	if err != nil {
		t.Fatalf("build upload task: %v", err)
	}
	return core.BackgroundTask{
		ID:          "task-test",
		OperationID: operationID,
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	}
}

func TestRunBackgroundUploadTaskMultiTaskUsesOnlyTaskScopedRecoveryState(t *testing.T) {
	operationID := "operation-multi"
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "already-rejected.jpg", Size: 12, TargetID: "default", Status: "error", Error: "rejected"}}}
	base := &recordingUploadWorkerStore{
		checkpoint:      backgroundUploadInitialCheckpoint(),
		found:           true,
		operationStatus: core.BackgroundWorkRunning,
		progressTotal:   2,
	}
	store := &recordingTaskUploadWorkerStore{
		recordingUploadWorkerStore: base,
		taskCheckpoint:             backgroundUploadInitialCheckpoint(),
		taskFound:                  true,
	}
	importer := &recordingUploadImporter{response: response}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTestTask(t, operationID)); err != nil {
		t.Fatalf("run multi-task upload: %v", err)
	}
	if base.operationCheckpointReads != 0 {
		t.Fatalf("operation checkpoint reads = %d, want 0", base.operationCheckpointReads)
	}
	wantEvents := []string{"task-checkpoint:activated", "task-checkpoint:imported", "task-result"}
	if !reflect.DeepEqual(store.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", store.events, wantEvents)
	}
	if !reflect.DeepEqual(store.taskResult, response) {
		t.Fatalf("task result = %#v, want %#v", store.taskResult, response)
	}
	if !reflect.DeepEqual(base.result, UploadImportResponse{}) {
		t.Fatalf("operation result unexpectedly changed: %#v", base.result)
	}
	if importer.state.taskID != "task-test" || importer.state.operationID != "" {
		t.Fatalf("multi-task import state = %+v, want task-only task-test", importer.state)
	}
}

func TestRunBackgroundUploadTaskMultiTaskMissingTaskCheckpointDoesNotFallback(t *testing.T) {
	base := &recordingUploadWorkerStore{
		checkpoint:      backgroundUploadInitialCheckpoint(),
		found:           true,
		operationStatus: core.BackgroundWorkRunning,
		progressTotal:   2,
	}
	store := &recordingTaskUploadWorkerStore{recordingUploadWorkerStore: base}
	importer := &recordingUploadImporter{}

	err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTestTask(t, "operation-missing-task-state"))
	if err == nil || !strings.Contains(err.Error(), "upload task checkpoint is missing") {
		t.Fatalf("run multi-task upload error = %v, want missing task checkpoint", err)
	}
	if base.operationCheckpointReads != 0 {
		t.Fatalf("operation checkpoint reads = %d, want 0", base.operationCheckpointReads)
	}
	if importer.calls != 0 {
		t.Fatalf("import calls = %d, want 0", importer.calls)
	}
	if len(store.events) != 0 {
		t.Fatalf("events = %#v, want none", store.events)
	}
}

func TestRunBackgroundUploadTaskSingleTaskFallsBackAndMirrorsTaskState(t *testing.T) {
	operationID := "operation-legacy-single"
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "already-rejected.jpg", Size: 12, TargetID: "default", Status: "error", Error: "rejected"}}}
	base := &recordingUploadWorkerStore{
		checkpoint:      backgroundUploadInitialCheckpoint(),
		found:           true,
		operationStatus: core.BackgroundWorkRunning,
		progressTotal:   1,
	}
	store := &recordingTaskUploadWorkerStore{recordingUploadWorkerStore: base}

	if err := runBackgroundUploadTask(context.Background(), &recordingUploadImporter{response: response}, store, backgroundUploadWorkerTestTask(t, operationID)); err != nil {
		t.Fatalf("run legacy single-task upload: %v", err)
	}
	if store.taskCheckpointReads != 1 {
		t.Fatalf("task checkpoint reads = %d, want 1", store.taskCheckpointReads)
	}
	if base.operationCheckpointReads != 1 {
		t.Fatalf("operation checkpoint reads = %d, want 1", base.operationCheckpointReads)
	}
	wantEvents := []string{
		"task-checkpoint:activated", "checkpoint:activated",
		"task-checkpoint:imported", "checkpoint:imported",
		"task-result", "result",
	}
	if !reflect.DeepEqual(store.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", store.events, wantEvents)
	}
	if !reflect.DeepEqual(store.taskResult, response) || !reflect.DeepEqual(base.result, response) {
		t.Fatalf("mirrored results = task %#v operation %#v, want %#v", store.taskResult, base.result, response)
	}
}

func TestRunBackgroundUploadTaskUsesOperationAwareImportBeforePublishingResult(t *testing.T) {
	operationID := "operation-test"
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "already-rejected.jpg", Size: 12, TargetID: "default", Status: "error", Error: "rejected"}}}
	importer := &recordingUploadImporter{response: response}
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true, operationStatus: core.BackgroundWorkRunning}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTestTask(t, operationID)); err != nil {
		t.Fatalf("run upload task: %v", err)
	}
	if importer.calls != 1 {
		t.Fatalf("import calls = %d, want 1", importer.calls)
	}
	if importer.state.operationID != operationID || importer.state.taskID != "" {
		t.Fatalf("legacy import state = %+v, want operation-only %q", importer.state, operationID)
	}
	wantEvents := []string{"checkpoint:activated", "checkpoint:imported", "result"}
	if !reflect.DeepEqual(store.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", store.events, wantEvents)
	}
	if !reflect.DeepEqual(store.result, response) {
		t.Fatalf("result = %#v, want %#v", store.result, response)
	}
}

func TestRunBackgroundUploadTaskReplaysImportedCheckpointWithoutReimport(t *testing.T) {
	operationID := "operation-test"
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "already-rejected.jpg", Size: 12, TargetID: "default", Status: "error", Error: "rejected"}}}
	importer := &recordingUploadImporter{}
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadImportedCheckpoint(nil, response), found: true, operationStatus: core.BackgroundWorkRunning}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTestTask(t, operationID)); err != nil {
		t.Fatalf("replay upload task: %v", err)
	}
	if importer.calls != 0 {
		t.Fatalf("import calls = %d, want 0", importer.calls)
	}
	wantEvents := []string{"result"}
	if !reflect.DeepEqual(store.events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", store.events, wantEvents)
	}
	if !reflect.DeepEqual(store.result, response) {
		t.Fatalf("result = %#v, want %#v", store.result, response)
	}
}

func TestRunBackgroundUploadTaskRollsBackClaimedReplacementWhenCanceledDuringImport(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "photo.jpg")
	stagedPath := filepath.Join(dir, ".photo.jpg.tmp-cancel")
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 3, targetID: "default", replace: true}}
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true, operationStatus: core.BackgroundWorkRunning}
	importer := &recordingUploadImporter{err: context.Canceled, before: func() { store.operationStatus = core.BackgroundWorkCanceled }}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTaskForFiles(t, "operation-cancel", files)); err != nil {
		t.Fatalf("run canceled upload task: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "old" {
		t.Fatalf("replacement after cancellation = %q, want original", got)
	}
	for _, path := range []string{stagedPath, stagedPath + ".backup", stagedPath + ".no-original", stagedPath + durableUploadActivatedMarkerSuffix} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("canceled replacement residue %q: %v", path, err)
		}
	}
}

func TestRunBackgroundUploadTaskPreservesClaimedReplacementForRetryableFailure(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "photo.jpg")
	stagedPath := filepath.Join(dir, ".photo.jpg.tmp-retry")
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 3, targetID: "default", replace: true}}
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true, operationStatus: core.BackgroundWorkRunning}
	importer := &recordingUploadImporter{err: errors.New("transient import failure")}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTaskForFiles(t, "operation-retry", files)); err == nil {
		t.Fatal("retryable import failure unexpectedly succeeded")
	}
	if got := string(mustReadFile(t, finalPath)); got != "new" {
		t.Fatalf("active replacement after retryable failure = %q, want new staged content", got)
	}
	if got := string(mustReadFile(t, stagedPath+".backup")); got != "old" {
		t.Fatalf("preserved replacement backup = %q, want old", got)
	}
	if got := string(mustReadFile(t, stagedPath+durableUploadActivatedMarkerSuffix)); got != "new" {
		t.Fatalf("preserved replacement recovery marker = %q, want new", got)
	}
	if store.checkpoint.Phase != backgroundUploadPhaseActivated {
		t.Fatalf("checkpoint phase = %q, want activated", store.checkpoint.Phase)
	}
}

func TestRunBackgroundUploadTaskRestoresProtectedReplacementAfterOpaqueCrashLoss(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "photo.jpg")
	stagedPath := filepath.Join(dir, ".photo.jpg.tmp-protected-crash")
	opaquePath := filepath.Join(dir, "opaque-orphan")
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 3, targetID: "default", replace: true}}
	task := backgroundUploadWorkerTaskForFiles(t, "operation-protected-crash", files)
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true, operationStatus: core.BackgroundWorkRunning}
	firstImporter := &recordingUploadImporter{
		err: errors.New("simulated process crash before database commit"),
		before: func() {
			if err := os.Rename(finalPath, opaquePath); err != nil {
				t.Fatalf("move activated replacement to opaque storage: %v", err)
			}
			if err := os.Remove(opaquePath); err != nil {
				t.Fatalf("simulate startup orphan cleanup: %v", err)
			}
		},
	}
	if err := runBackgroundUploadTask(context.Background(), firstImporter, store, task); err == nil {
		t.Fatal("simulated pre-commit crash unexpectedly succeeded")
	}
	if store.checkpoint.Phase != backgroundUploadPhaseActivated {
		t.Fatalf("checkpoint phase = %q, want activated", store.checkpoint.Phase)
	}
	if _, err := os.Stat(finalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logical replacement survived simulated opaque cleanup: %v", err)
	}
	if got := string(mustReadFile(t, stagedPath+durableUploadActivatedMarkerSuffix)); got != "new" {
		t.Fatalf("recovery marker after opaque cleanup = %q, want new", got)
	}
	if got := string(mustReadFile(t, stagedPath+".backup")); got != "old" {
		t.Fatalf("replacement backup after opaque cleanup = %q, want old", got)
	}

	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "photo.jpg", Size: 3, TargetID: "default", Status: "imported"}}}
	secondImporter := &recordingUploadImporter{response: response, before: func() {
		if got := string(mustReadFile(t, finalPath)); got != "new" {
			t.Fatalf("restored replacement before retry import = %q, want new", got)
		}
	}}
	if err := runBackgroundUploadTask(context.Background(), secondImporter, store, task); err != nil {
		t.Fatalf("retry after protected opaque crash loss: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "new" {
		t.Fatalf("replacement after successful retry = %q, want new", got)
	}
	for _, path := range []string{stagedPath + ".backup", stagedPath + ".no-original", stagedPath + durableUploadActivatedMarkerSuffix} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("successful retry residue %q: %v", path, err)
		}
	}
}

func TestRunBackgroundUploadTaskResumesReplacementActivatedBeforeCheckpoint(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "photo.jpg")
	stagedPath := filepath.Join(dir, ".photo.jpg.tmp-partial-activation")
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 3, targetID: "default", replace: true}}
	if err := prepareDurableReplacementRecoveryMarkers(files); err != nil {
		t.Fatalf("prepare recovery marker: %v", err)
	}
	if _, err := activateReplacement(stagedPath, finalPath); err != nil {
		t.Fatalf("simulate activation before checkpoint: %v", err)
	}
	if _, err := os.Stat(stagedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged path after simulated activation: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "new" {
		t.Fatalf("simulated activated destination = %q, want new", got)
	}

	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "photo.jpg", Size: 3, TargetID: "default", Status: "imported"}}}
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true, operationStatus: core.BackgroundWorkRunning}
	if err := runBackgroundUploadTask(context.Background(), &recordingUploadImporter{response: response}, store, backgroundUploadWorkerTaskForFiles(t, "operation-partial-activation", files)); err != nil {
		t.Fatalf("resume activation before checkpoint: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "new" {
		t.Fatalf("replacement after resumed activation = %q, want new", got)
	}
	for _, path := range []string{stagedPath, stagedPath + ".backup", stagedPath + ".no-original", stagedPath + durableUploadActivatedMarkerSuffix} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("resumed activation residue %q: %v", path, err)
		}
	}
}

func TestRunBackgroundUploadTaskRollsBackActivationWhenCancelWinsCheckpointRace(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "photo.jpg")
	stagedPath := filepath.Join(dir, ".photo.jpg.tmp-checkpoint")
	if err := os.WriteFile(finalPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 3, targetID: "default", replace: true}}
	store := &recordingUploadWorkerStore{
		checkpoint:      backgroundUploadInitialCheckpoint(),
		found:           true,
		operationStatus: core.BackgroundWorkCanceled,
		checkpointErr:   errors.New("background operation is missing or no longer active"),
	}

	if err := runBackgroundUploadTask(context.Background(), &recordingUploadImporter{}, store, backgroundUploadWorkerTaskForFiles(t, "operation-checkpoint-cancel", files)); err != nil {
		t.Fatalf("run checkpoint-race cancellation: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "old" {
		t.Fatalf("replacement after checkpoint-race cancellation = %q, want original", got)
	}
	for _, path := range []string{stagedPath, stagedPath + ".backup", stagedPath + ".no-original", stagedPath + durableUploadActivatedMarkerSuffix} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("checkpoint-race residue %q: %v", path, err)
		}
	}
}
