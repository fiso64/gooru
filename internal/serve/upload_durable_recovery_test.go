package serve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDurableStagedUploadsUsesActivatedDestinationForAnalysis(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".gooru-upload-staging", "operation", "photo.jpg")
	destinationPath := filepath.Join(root, "photo.jpg")
	files := []savedUpload{{
		name:            "photo.jpg",
		path:            stagedPath,
		destinationPath: destinationPath,
		targetID:        "default",
	}}

	staged := durableStagedUploads(files)
	if len(staged) != 1 {
		t.Fatalf("staged uploads = %d, want 1", len(staged))
	}
	if staged[0].Path != destinationPath {
		t.Fatalf("import path = %q, want activated destination %q", staged[0].Path, destinationPath)
	}
	if staged[0].AnalysisPath != destinationPath {
		t.Fatalf("analysis path = %q, want activated destination %q", staged[0].AnalysisPath, destinationPath)
	}
}

func TestActivateDurableUploadDestinationDropsStaleMarkerOnConflictingReplay(t *testing.T) {
	root := t.TempDir()
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequestWithConflict(t, map[string]string{"photo.jpg": "staged"}, nil, "", "error")
	_, saved, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	markerPath := saved[0].path + durableUploadActivatedMarkerSuffix
	if err := os.WriteFile(markerPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved[0].destinationPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := activateSavedDurableUploads(saved); !errors.Is(err, errUploadConflict) {
		t.Fatalf("activation error = %v, want upload conflict", err)
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale activation marker remained after conflict: %v", err)
	}
	if err := rollbackDurableNonreplacementActivations(saved); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadFile(t, saved[0].destinationPath)); got != "racer" {
		t.Fatalf("rollback changed racing destination to %q", got)
	}
}

func TestCleanupCanceledClaimedUploadPropagatesRemovalFailure(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, "blocked-stage")
	if err := os.Mkdir(stagedPath, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(stagedPath, "still-present")
	if err := os.WriteFile(child, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{name: "blocked-stage", path: stagedPath, destinationPath: stagedPath, targetID: "default"}}

	err := cleanupCanceledClaimedUpload(files)
	if err == nil {
		t.Fatal("cleanup unexpectedly succeeded while staged path was non-empty")
	}
	if !strings.Contains(err.Error(), "remove canceled staged uploads") {
		t.Fatalf("cleanup error = %v, want retriable staged-removal failure", err)
	}
	if _, statErr := os.Stat(stagedPath); statErr != nil {
		t.Fatalf("failed cleanup removed staged path unexpectedly: %v", statErr)
	}
}
