package serve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
)

func TestRunBackgroundUploadTaskRestoresProtectedNonreplacementAfterOpaqueCrashLoss(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, ".gooru-upload-staging", "operation-protected-crash", "photo.jpg")
	finalPath := filepath.Join(dir, "photo.jpg")
	opaquePath := filepath.Join(dir, "opaque-orphan")
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 3, targetID: "default"}}
	task := backgroundUploadWorkerTaskForFiles(t, "operation-protected-nonreplace-crash", files)
	store := &recordingUploadWorkerStore{checkpoint: backgroundUploadInitialCheckpoint(), found: true, operationStatus: core.BackgroundWorkRunning}
	firstImporter := &recordingUploadImporter{
		err: errors.New("simulated process crash before database commit"),
		before: func() {
			if err := os.Rename(finalPath, opaquePath); err != nil {
				t.Fatalf("move activated upload to opaque storage: %v", err)
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
		t.Fatalf("logical upload survived simulated opaque cleanup: %v", err)
	}
	if _, err := os.Stat(stagedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged upload remained after activation: %v", err)
	}
	markerPath := stagedPath + durableUploadActivatedMarkerSuffix
	if got := string(mustReadFile(t, markerPath)); got != "new" {
		t.Fatalf("activation marker after opaque cleanup = %q, want new", got)
	}

	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "photo.jpg", Size: 3, TargetID: "default", Status: "imported"}}}
	secondImporter := &recordingUploadImporter{response: response, before: func() {
		if got := string(mustReadFile(t, finalPath)); got != "new" {
			t.Fatalf("restored upload before retry import = %q, want new", got)
		}
	}}
	if err := runBackgroundUploadTask(context.Background(), secondImporter, store, task); err != nil {
		t.Fatalf("retry after protected opaque crash loss: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "new" {
		t.Fatalf("upload after successful retry = %q, want new", got)
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful retry left activation marker: %v", err)
	}
}

func TestRestoreDurableNonreplacementDestinationsPreservesRacingDestination(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, ".gooru-upload-staging", "operation-race", "photo.jpg")
	finalPath := filepath.Join(dir, "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("uploaded"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: finalPath, size: 8, targetID: "default"}}
	if err := activateSavedDurableUploads(files); err != nil {
		t.Fatalf("activate durable upload: %v", err)
	}
	if err := os.Remove(finalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("racer"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := restoreDurableNonreplacementDestinations(files)
	if !errors.Is(err, errUploadConflict) {
		t.Fatalf("restore durable destination error = %v, want upload conflict", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "racer" {
		t.Fatalf("racing destination after restore = %q, want racer", got)
	}
	if got := string(mustReadFile(t, stagedPath+durableUploadActivatedMarkerSuffix)); got != "uploaded" {
		t.Fatalf("recovery marker after conflict = %q, want uploaded", got)
	}
}
