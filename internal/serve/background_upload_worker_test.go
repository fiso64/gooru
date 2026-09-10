package serve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	core "gooru.local/gooru"
)

type recordingUploadImporter struct {
	calls       int
	response    UploadImportResponse
	err         error
	operationID string
	activated   int
	before      func()
}

func (i *recordingUploadImporter) importUploadedFiles(_ context.Context, _ []StagedUpload, _ []string, operationID string, activated []activatedSavedReplacement) (UploadImportResponse, error) {
	i.calls++
	i.operationID = operationID
	i.activated = len(activated)
	if i.before != nil {
		i.before()
	}
	return i.response, i.err
}

type recordingUploadWorkerStore struct {
	checkpoint      backgroundUploadCheckpoint
	found           bool
	result          UploadImportResponse
	events          []string
	operationStatus core.BackgroundWorkStatus
	checkpointErr   error
	resultErr       error
}

func (s *recordingUploadWorkerStore) GetBackgroundOperation(operationID string) (core.BackgroundOperationState, bool, error) {
	return core.BackgroundOperationState{ID: operationID, Kind: backgroundUploadImportOperationKind, Status: s.operationStatus}, true, nil
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
	if importer.operationID != operationID {
		t.Fatalf("operation-aware import id = %q, want %q", importer.operationID, operationID)
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
	for _, path := range []string{stagedPath, stagedPath + ".backup", stagedPath + ".no-original"} {
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
	if store.checkpoint.Phase != backgroundUploadPhaseActivated {
		t.Fatalf("checkpoint phase = %q, want activated", store.checkpoint.Phase)
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
	for _, path := range []string{stagedPath, stagedPath + ".backup", stagedPath + ".no-original"} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("checkpoint-race residue %q: %v", path, err)
		}
	}
}
