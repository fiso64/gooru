package serve

import (
	"context"
	"reflect"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type recordingUploadImporter struct {
	calls    int
	response UploadImportResponse
	err      error
	state    backgroundUploadImportState
	before   func()
}

func (i *recordingUploadImporter) importUploadedFiles(_ context.Context, _ []StagedUpload, _ []string, state backgroundUploadImportState) (UploadImportResponse, error) {
	i.calls++
	i.state = state
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
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadImportedCheckpoint(response), found: true, operationStatus: core.BackgroundWorkRunning}

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
