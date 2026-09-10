package serve

import (
	"context"
	"reflect"
	"testing"

	core "gooru.local/gooru"
)

type recordingUploadImporter struct {
	calls    int
	response UploadImportResponse
	err      error
}

func (i *recordingUploadImporter) ImportUploadedFiles(context.Context, []StagedUpload, []string) (UploadImportResponse, error) {
	i.calls++
	return i.response, i.err
}

type recordingUploadWorkerStore struct {
	checkpoint backgroundUploadCheckpoint
	found      bool
	result     UploadImportResponse
	events     []string
}

func (s *recordingUploadWorkerStore) GetBackgroundOperationCheckpoint(_ string, destination any) (bool, error) {
	if !s.found {
		return false, nil
	}
	checkpoint := destination.(*backgroundUploadCheckpoint)
	*checkpoint = s.checkpoint
	return true, nil
}

func (s *recordingUploadWorkerStore) SetBackgroundOperationCheckpoint(_ string, checkpoint any) error {
	s.checkpoint = checkpoint.(backgroundUploadCheckpoint)
	s.found = true
	s.events = append(s.events, "checkpoint:"+s.checkpoint.Phase)
	return nil
}

func (s *recordingUploadWorkerStore) SetBackgroundOperationResult(_ string, result any) error {
	s.result = result.(UploadImportResponse)
	s.events = append(s.events, "result")
	return nil
}

func backgroundUploadWorkerTestTask(t *testing.T, operationID string) core.BackgroundTask {
	t.Helper()
	request, err := backgroundUploadTaskRequest(operationID, []savedUpload{{
		name:     "already-rejected.jpg",
		size:     12,
		targetID: "default",
		status:   "error",
		error:    "rejected",
	}}, []string{"artist:test"})
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

func TestRunBackgroundUploadTaskPersistsImportedCheckpointBeforeResult(t *testing.T) {
	operationID := "operation-test"
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "already-rejected.jpg", Size: 12, TargetID: "default", Status: "error", Error: "rejected"}}}
	importer := &recordingUploadImporter{response: response}
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTestTask(t, operationID)); err != nil {
		t.Fatalf("run upload task: %v", err)
	}
	if importer.calls != 1 {
		t.Fatalf("import calls = %d, want 1", importer.calls)
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
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadImportedCheckpoint(nil, response), found: true}

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
