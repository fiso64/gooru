package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
)

type terminalUploadCleanupCheckpointStore struct {
	checkpoint backgroundUploadCheckpoint
}

func (s terminalUploadCleanupCheckpointStore) GetBackgroundOperationTask(string) (core.BackgroundTaskState, bool, error) {
	return core.BackgroundTaskState{}, false, nil
}

func (s terminalUploadCleanupCheckpointStore) GetBackgroundOperationCheckpoint(_ string, output any) (bool, error) {
	checkpoint, ok := output.(*backgroundUploadCheckpoint)
	if !ok {
		return false, nil
	}
	*checkpoint = s.checkpoint
	return true, nil
}

func TestCleanupCanceledImportedUploadSettlesReplacementRecoveryMarker(t *testing.T) {
	operationID := "operation-terminal-cleanup"
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".photo.jpg.tmp")
	destinationPath := filepath.Join(root, "photo.jpg")
	markerPath := stagedPath + durableUploadActivatedMarkerSuffix

	if err := os.WriteFile(destinationPath, []byte("replacement"), 0600); err != nil {
		t.Fatalf("write replacement destination: %v", err)
	}
	if err := os.Link(destinationPath, markerPath); err != nil {
		t.Fatalf("create replacement recovery marker: %v", err)
	}

	files := []savedUpload{{
		name:            "photo.jpg",
		path:            stagedPath,
		destinationPath: destinationPath,
		size:            int64(len("replacement")),
		targetID:        "default",
		replace:         true,
	}}
	request, err := backgroundUploadTaskRequest(operationID, files, nil)
	if err != nil {
		t.Fatalf("backgroundUploadTaskRequest: %v", err)
	}
	task := core.BackgroundTask{
		ID:          "task-terminal-cleanup",
		OperationID: operationID,
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	}
	response := UploadImportResponse{Files: []UploadedFileDTO{{
		Name:     "photo.jpg",
		TargetID: "default",
		Status:   "imported",
	}}}
	activated := []activatedSavedReplacement{{
		index: 0,
		replacement: activatedReplacement{
			finalPath:            destinationPath,
			backupPath:           stagedPath + ".backup",
			noOriginalMarkerPath: stagedPath + ".no-original",
		},
	}}
	store := terminalUploadCleanupCheckpointStore{
		checkpoint: backgroundUploadImportedCheckpoint(activated, response),
	}

	if err := cleanupCanceledDurableUpload(store, operationID, task); err != nil {
		t.Fatalf("cleanupCanceledDurableUpload: %v", err)
	}
	if _, err := os.Lstat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("replacement recovery marker remains after terminal cleanup: %v", err)
	}
	contents, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("read replacement destination: %v", err)
	}
	if string(contents) != "replacement" {
		t.Fatalf("replacement destination contents = %q", contents)
	}
}

func TestCleanupCanceledImportedUploadPreservesRacingReplacementDestination(t *testing.T) {
	operationID := "operation-terminal-cleanup-race"
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".photo.jpg.tmp")
	destinationPath := filepath.Join(root, "photo.jpg")
	backupPath := stagedPath + ".backup"
	markerPath := stagedPath + durableUploadActivatedMarkerSuffix

	if err := os.WriteFile(destinationPath, []byte("racer"), 0600); err != nil {
		t.Fatalf("write racing destination: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write preserved original: %v", err)
	}
	if err := os.WriteFile(markerPath, []byte("replacement"), 0600); err != nil {
		t.Fatalf("write replacement recovery marker: %v", err)
	}

	files := []savedUpload{{
		name:            "photo.jpg",
		path:            stagedPath,
		destinationPath: destinationPath,
		size:            int64(len("replacement")),
		targetID:        "default",
		replace:         true,
	}}
	request, err := backgroundUploadTaskRequest(operationID, files, nil)
	if err != nil {
		t.Fatalf("backgroundUploadTaskRequest: %v", err)
	}
	task := core.BackgroundTask{
		ID:          "task-terminal-cleanup-race",
		OperationID: operationID,
		Kind:        request.Kind,
		SubjectKind: request.SubjectKind,
		SubjectID:   request.SubjectID,
		InputKey:    request.InputKey,
	}
	response := UploadImportResponse{Files: []UploadedFileDTO{{
		Name:     "photo.jpg",
		TargetID: "default",
		Status:   "error",
	}}}
	activated := []activatedSavedReplacement{{
		index: 0,
		replacement: activatedReplacement{
			finalPath:            destinationPath,
			backupPath:           backupPath,
			noOriginalMarkerPath: stagedPath + ".no-original",
			hadOriginal:          true,
		},
	}}
	store := terminalUploadCleanupCheckpointStore{
		checkpoint: backgroundUploadImportedCheckpoint(activated, response),
	}

	if err := cleanupCanceledDurableUpload(store, operationID, task); !errors.Is(err, errUploadConflict) {
		t.Fatalf("cleanupCanceledDurableUpload error = %v, want upload conflict", err)
	}
	if contents, err := os.ReadFile(destinationPath); err != nil || string(contents) != "racer" {
		t.Fatalf("racing destination = %q, %v", contents, err)
	}
	if contents, err := os.ReadFile(backupPath); err != nil || string(contents) != "original" {
		t.Fatalf("preserved original = %q, %v", contents, err)
	}
	if contents, err := os.ReadFile(markerPath); err != nil || string(contents) != "replacement" {
		t.Fatalf("replacement recovery marker = %q, %v", contents, err)
	}
}
