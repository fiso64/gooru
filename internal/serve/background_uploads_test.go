package serve

import (
	"reflect"
	"strings"
	"testing"
	"time"

	core "gooru.local/gooru"
)

func TestBackgroundUploadTaskRoundTripsStagedExecutionState(t *testing.T) {
	sourceModTime := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	addedAt := time.Date(2024, 2, 4, 5, 6, 7, 0, time.UTC)
	files := []savedUpload{
		{
			name:            "replace.png",
			path:            "/uploads/.replace.png.tmp-1",
			destinationPath: "/uploads/replace.png",
			size:            123,
			targetID:        "default",
			sourceModTime:   sourceModTime,
			addedAt:         addedAt,
			conflictPolicy:  "rename",
		},
		{
			name:     "bad.png",
			size:     45,
			targetID: "default",
			status:   "error",
			error:    "invalid image",
		},
	}
	tags := []string{"artist:one", "rating:safe"}
	request, err := backgroundUploadTaskRequest("operation-1", files, tags)
	if err != nil {
		t.Fatalf("backgroundUploadTaskRequest: %v", err)
	}
	if request.Kind != backgroundUploadTaskKind || request.ResourceClass != backgroundUploadResourceClass || request.SubjectID != "operation-1" {
		t.Fatalf("unexpected task request: %+v", request)
	}

	decodedFiles, decodedTags, err := decodeBackgroundUploadTask(core.BackgroundTask{
		ID:          "task-1",
		OperationID: "operation-1",
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	})
	if err != nil {
		t.Fatalf("decodeBackgroundUploadTask: %v", err)
	}
	if !reflect.DeepEqual(decodedFiles, files) {
		t.Fatalf("decoded files = %#v, want %#v", decodedFiles, files)
	}
	if !reflect.DeepEqual(decodedTags, tags) {
		t.Fatalf("decoded tags = %#v, want %#v", decodedTags, tags)
	}
}

func TestBackgroundUploadTaskAllowsUnnamedPerFileError(t *testing.T) {
	files := []savedUpload{{targetID: "default", status: "error", error: "uploaded filename is invalid"}}
	request, err := backgroundUploadTaskRequest("operation-1", files, nil)
	if err != nil {
		t.Fatalf("backgroundUploadTaskRequest: %v", err)
	}
	decodedFiles, _, err := decodeBackgroundUploadTask(core.BackgroundTask{
		OperationID: "operation-1",
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	})
	if err != nil {
		t.Fatalf("decodeBackgroundUploadTask: %v", err)
	}
	if !reflect.DeepEqual(decodedFiles, files) {
		t.Fatalf("decoded files = %#v, want %#v", decodedFiles, files)
	}
}

func TestBackgroundUploadTaskRejectsInvalidPersistedState(t *testing.T) {
	if _, err := backgroundUploadTaskRequest("operation-1", nil, nil); err == nil {
		t.Fatal("empty upload payload unexpectedly accepted")
	}
	request, err := backgroundUploadTaskRequest("operation-1", []savedUpload{{name: "a.png", path: "/uploads/a.png", destinationPath: "/uploads/a.png", targetID: "default"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeBackgroundUploadTask(core.BackgroundTask{
		OperationID: "operation-1",
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   "different-operation",
		InputKey:    request.InputKey,
	}); err == nil {
		t.Fatal("mismatched operation identity unexpectedly accepted")
	}
}

func TestBackgroundUploadTaskRejectsLegacyReplacementState(t *testing.T) {
	const operationID = "operation-legacy-replace"
	inputs := []string{
		`{"version":1,"files":[{"name":"photo.jpg","path":"/uploads/.photo.jpg.tmp","destination_path":"/uploads/photo.jpg","size":3,"target_id":"default","replace":true}]}`,
		`{"version":1,"files":[{"name":"photo.jpg","path":"/uploads/.photo.jpg.tmp","destination_path":"/uploads/photo.jpg","size":3,"target_id":"default","conflict_policy":"replace"}]}`,
	}
	for _, input := range inputs {
		_, _, err := decodeBackgroundUploadTask(core.BackgroundTask{
			OperationID: operationID,
			Kind:        backgroundUploadTaskKind,
			SubjectKind: "operation",
			SubjectID:   operationID,
			InputKey:    input,
		})
		if err == nil || !strings.Contains(err.Error(), "removed replace conflict policy") {
			t.Fatalf("legacy replacement state error = %v, want explicit removed-policy rejection", err)
		}
	}
}

func TestBackgroundUploadInitialCheckpointStartsAtStagedPhase(t *testing.T) {
	checkpoint := backgroundUploadInitialCheckpoint()
	if checkpoint.Phase != backgroundUploadPhaseStaged {
		t.Fatalf("checkpoint phase = %q, want %q", checkpoint.Phase, backgroundUploadPhaseStaged)
	}
}

func TestBackgroundUploadTaskAcceptsMoreThanOneThousandFiles(t *testing.T) {
	files := make([]savedUpload, 1001)
	for i := range files {
		files[i] = savedUpload{name: "a.jpg", path: "/uploads/a.jpg", targetID: "default"}
	}
	if _, err := backgroundUploadTaskRequest("operation-1001", files, nil); err != nil {
		t.Fatalf("upload task above 1K rejected: %v", err)
	}
}

func TestBackgroundUploadCheckpointCarriesFileProgress(t *testing.T) {
	checkpoint := backgroundUploadActivatedCheckpoint(1000, 420, 417)
	if checkpoint.FileTotal != 1000 || checkpoint.FilesCompleted != 420 || checkpoint.FilesCompletedPrefix != 417 {
		t.Fatalf("checkpoint progress = %+v", checkpoint)
	}
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 1000)}
	checkpoint = backgroundUploadImportedCheckpoint(response)
	if checkpoint.FileTotal != 1000 || checkpoint.FilesCompleted != 1000 || checkpoint.FilesCompletedPrefix != 1000 {
		t.Fatalf("terminal checkpoint progress = %+v", checkpoint)
	}
}
