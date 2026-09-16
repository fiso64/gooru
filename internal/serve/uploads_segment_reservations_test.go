package serve

import (
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
)

type uploadSegmentReservationTestStore struct {
	tasks map[string]core.BackgroundTaskState
}

func (s uploadSegmentReservationTestStore) GetBackgroundTask(taskID string) (core.BackgroundTaskState, bool, error) {
	task, found := s.tasks[taskID]
	return task, found, nil
}

func (uploadSegmentReservationTestStore) AttachBackgroundTaskToOperation(string, string, any, core.BackgroundTaskRequest) (core.BackgroundTask, bool, error) {
	panic("unexpected AttachBackgroundTaskToOperation call")
}

func TestDurableUploadPriorSegmentDestinationsReservePlannedRename(t *testing.T) {
	operationID := "operation-segment-reservations"
	targetDir := t.TempDir()
	planned := filepath.Join(targetDir, "photo.jpg")
	request, err := backgroundUploadTaskRequest(operationID, []savedUpload{{
		name:            "photo.jpg",
		path:            filepath.Join(t.TempDir(), "photo.jpg"),
		destinationPath: planned,
		targetID:        "default",
	}}, nil)
	if err != nil {
		t.Fatalf("build prior task input: %v", err)
	}
	taskID := durableUploadSegmentTaskID(operationID, 0)
	store := uploadSegmentReservationTestStore{tasks: map[string]core.BackgroundTaskState{
		taskID: {
			BackgroundTask: core.BackgroundTask{
				ID:          taskID,
				OperationID: operationID,
				Kind:        backgroundUploadTaskKind,
				SubjectKind: "operation",
				SubjectID:   operationID,
				InputKey:    request.InputKey,
			},
		},
	}}

	reserved, err := durableUploadPriorSegmentDestinations(store, operationID, 1)
	if err != nil {
		t.Fatalf("load prior segment destinations: %v", err)
	}
	if _, ok := reserved[planned]; !ok {
		t.Fatalf("reserved destinations = %#v, want %q", reserved, planned)
	}

	path, err := chooseDurableUploadDestination(targetDir, "photo.jpg", "rename", reserved)
	if err != nil {
		t.Fatalf("choose later segment destination: %v", err)
	}
	want := filepath.Join(targetDir, "photo-1.jpg")
	if path != want {
		t.Fatalf("later destination = %q, want %q", path, want)
	}
}

func TestDurableUploadPriorSegmentDestinationsToleratesUnadmittedSlots(t *testing.T) {
	reserved, err := durableUploadPriorSegmentDestinations(uploadSegmentReservationTestStore{tasks: map[string]core.BackgroundTaskState{}}, "operation-segment-reservations", 3)
	if err != nil {
		t.Fatalf("load prior segment destinations: %v", err)
	}
	if len(reserved) != 0 {
		t.Fatalf("reserved destinations = %#v, want empty", reserved)
	}
}
