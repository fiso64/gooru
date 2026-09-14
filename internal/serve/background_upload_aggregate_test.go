package serve

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	core "gooru.local/gooru"
)

type fakeSegmentedUploadReader struct {
	*fakeBackgroundOperationReader
	tasks           map[string]core.BackgroundTaskState
	taskCheckpoints map[string]backgroundUploadCheckpoint
	taskResults     map[string]UploadImportResponse
}

func (f *fakeSegmentedUploadReader) GetBackgroundTask(id string) (core.BackgroundTaskState, bool, error) {
	task, ok := f.tasks[id]
	return task, ok, nil
}

func (f *fakeSegmentedUploadReader) GetBackgroundTaskCheckpoint(id string, destination any) (bool, error) {
	checkpoint, ok := f.taskCheckpoints[id]
	if !ok {
		return false, nil
	}
	*(destination.(*backgroundUploadCheckpoint)) = checkpoint
	return true, nil
}

func (f *fakeSegmentedUploadReader) GetBackgroundTaskResult(id string, destination any) (bool, error) {
	result, ok := f.taskResults[id]
	if !ok {
		return false, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(encoded, destination); err != nil {
		return false, err
	}
	return true, nil
}

func TestHandleOperationAggregatesCompletedSegmentedUploadResultInOrder(t *testing.T) {
	operationID := "operation-segmented-result"
	finishedAt := time.Now().UTC()
	operation := core.BackgroundOperationState{
		ID: operationID, Kind: backgroundUploadImportOperationKind, Visible: true,
		Status: core.BackgroundWorkCompleted, ProgressTotal: 3,
		ProgressCompleted: 3, CreatedAt: finishedAt.Add(-time.Second), FinishedAt: &finishedAt,
	}
	base := &fakeBackgroundOperationReader{
		byID: map[string]core.BackgroundOperationState{operationID: operation},
		checkpoints: map[string]backgroundUploadCheckpoint{
			operationID: backgroundUploadReceivingCheckpoint(100, 100),
		},
	}
	reader := &fakeSegmentedUploadReader{
		fakeBackgroundOperationReader: base,
		tasks:                           map[string]core.BackgroundTaskState{},
		taskCheckpoints:                 map[string]backgroundUploadCheckpoint{},
		taskResults:                     map[string]UploadImportResponse{},
	}
	segmentFiles := [][]UploadedFileDTO{
		{{Name: "a.jpg", TargetID: "default", Status: "imported"}, {Name: "b.jpg", TargetID: "default", Status: "duplicate_existing"}},
		{{Name: "c.jpg", TargetID: "default", Status: "imported"}},
		{{Name: "d.jpg", TargetID: "default", Status: "imported"}},
	}
	for segmentIndex, files := range segmentFiles {
		index := int64(segmentIndex)
		taskID := durableUploadSegmentTaskID(operationID, index)
		reader.tasks[taskID] = segmentedUploadTestTask(operationID, index, core.BackgroundWorkCompleted)
		reader.taskCheckpoints[taskID] = backgroundUploadCheckpoint{
			Phase: backgroundUploadPhaseImported, FileTotal: len(files), FilesCompleted: len(files), FilesCompletedPrefix: len(files),
		}
		affected := 0
		for _, file := range files {
			if file.Status == "imported" {
				affected++
			}
		}
		reader.taskResults[taskID] = UploadImportResponse{
			Files: files, AffectedCount: affected, Notifications: make([]NotificationDTO, segmentIndex%2),
		}
	}

	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations/"+operationID, nil)
	response := httptest.NewRecorder()
	server.handleOperation(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Result UploadImportResponse `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Result.AffectedCount != 3 || len(payload.Result.Files) != 4 {
		t.Fatalf("aggregate result = %+v", payload.Result)
	}
	wantNames := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}
	for index, want := range wantNames {
		if got := payload.Result.Files[index].Name; got != want {
			t.Fatalf("result file %d = %q, want %q", index, got, want)
		}
	}
	if len(payload.Result.Notifications) != 1 {
		t.Fatalf("notifications = %d, want 1", len(payload.Result.Notifications))
	}
}

func TestHandleOperationsProjectsSegmentedUploadAcrossAdmittedAndMissingChildren(t *testing.T) {
	operationID := "operation-segmented-progress"
	operation := core.BackgroundOperationState{
		ID: operationID, Kind: backgroundUploadImportOperationKind, Visible: true,
		Status: core.BackgroundWorkRunning, ProgressTotal: 3, ProgressCompleted: 1,
		CreatedAt: time.Now().UTC(),
	}
	base := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{operation},
		checkpoints: map[string]backgroundUploadCheckpoint{
			operationID: backgroundUploadReceivingCheckpoint(400, 100),
		},
	}
	reader := &fakeSegmentedUploadReader{
		fakeBackgroundOperationReader: base,
		tasks:                           map[string]core.BackgroundTaskState{},
		taskCheckpoints:                 map[string]backgroundUploadCheckpoint{},
		taskResults:                     map[string]UploadImportResponse{},
	}
	task0 := durableUploadSegmentTaskID(operationID, 0)
	reader.tasks[task0] = segmentedUploadTestTask(operationID, 0, core.BackgroundWorkCompleted)
	reader.taskCheckpoints[task0] = backgroundUploadCheckpoint{Phase: backgroundUploadPhaseImported, FileTotal: 2, FilesCompleted: 2, FilesCompletedPrefix: 2}
	task2 := durableUploadSegmentTaskID(operationID, 2)
	reader.tasks[task2] = segmentedUploadTestTask(operationID, 2, core.BackgroundWorkRunning)
	reader.taskCheckpoints[task2] = backgroundUploadCheckpoint{Phase: backgroundUploadPhaseActivated, FileTotal: 4, FilesCompleted: 2, FilesCompletedPrefix: 1}

	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	response := httptest.NewRecorder()
	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %+v", payload.Items)
	}
	item := payload.Items[0]
	if item.Stage != "receiving" || item.Progress == nil || math.Abs(*item.Progress-0.625) > 1e-9 {
		t.Fatalf("partial segmented projection = %+v", item)
	}
	if item.ProgressTotal != 3 || item.ProgressCompleted != 1 {
		t.Fatalf("partial progress counters = %+v", item)
	}

	task1 := durableUploadSegmentTaskID(operationID, 1)
	reader.tasks[task1] = segmentedUploadTestTask(operationID, 1, core.BackgroundWorkPending)
	reader.taskCheckpoints[task1] = backgroundUploadCheckpoint{Phase: backgroundUploadPhaseStaged, FileTotal: 4}
	projected, ok := server.segmentedUploadOperationDTO(operation, backgroundOperationDTO(operation))
	if !ok {
		t.Fatal("all-admitted segmented upload was not projected")
	}
	if projected.Stage != "importing" || projected.Progress == nil || math.Abs(*projected.Progress-0.7) > 1e-9 {
		t.Fatalf("all-admitted segmented projection = %+v", projected)
	}
	if projected.ProgressTotal != 10 || projected.ProgressCompleted != 4 || projected.ProgressCompletedPrefix != 2 {
		t.Fatalf("all-admitted file counters = %+v", projected)
	}
}

func segmentedUploadTestTask(operationID string, segmentIndex int64, status core.BackgroundWorkStatus) core.BackgroundTaskState {
	taskID := durableUploadSegmentTaskID(operationID, segmentIndex)
	return core.BackgroundTaskState{
		BackgroundTask: core.BackgroundTask{
			ID: taskID, OperationID: operationID, Kind: backgroundUploadTaskKind,
			SubjectKind: "operation", SubjectID: operationID,
		},
		Status: status,
	}
}
