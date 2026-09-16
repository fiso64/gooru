package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestActivateSavedDurableUploadsRollsBackEarlierNonreplacementOnLaterConflict(t *testing.T) {
	root := t.TempDir()
	stagingDir := filepath.Join(root, durableUploadStagingRootName, "operation-test")
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		t.Fatal(err)
	}

	firstStaged := filepath.Join(stagingDir, "first.txt")
	secondStaged := filepath.Join(stagingDir, "second.txt")
	firstDestination := filepath.Join(root, "first.txt")
	secondDestination := filepath.Join(root, "second.txt")
	if err := os.WriteFile(firstStaged, []byte("first upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondStaged, []byte("second upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondDestination, []byte("racing destination"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := []savedUpload{
		{name: "first.txt", path: firstStaged, destinationPath: firstDestination, conflictPolicy: "error"},
		{name: "second.txt", path: secondStaged, destinationPath: secondDestination, conflictPolicy: "error"},
	}
	err := activateSavedDurableUploads(files)
	if !errors.Is(err, errUploadConflict) {
		t.Fatalf("activate durable uploads error = %v, want upload conflict", err)
	}

	if _, err := os.Stat(firstDestination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first destination remained after batch rollback: %v", err)
	}
	if got, err := os.ReadFile(firstStaged); err != nil || string(got) != "first upload" {
		t.Fatalf("first staged upload after rollback = %q, %v", got, err)
	}
	if _, err := os.Stat(firstStaged + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first activation marker remained after rollback: %v", err)
	}
	if got, err := os.ReadFile(secondDestination); err != nil || string(got) != "racing destination" {
		t.Fatalf("conflicting destination after rollback = %q, %v", got, err)
	}
	if got, err := os.ReadFile(secondStaged); err != nil || string(got) != "second upload" {
		t.Fatalf("second staged upload after rollback = %q, %v", got, err)
	}
	if _, err := os.Stat(secondStaged + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second activation marker remained after conflict: %v", err)
	}
}

func TestRestoreDurableNonreplacementActivationsPreservesRacingDestination(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".stage", "photo.jpg")
	destinationPath := filepath.Join(root, "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("uploaded"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := activateDurableUploadDestination(stagedPath, destinationPath); err != nil {
		t.Fatalf("activate durable destination: %v", err)
	}
	if err := os.Remove(destinationPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, []byte("racer"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: destinationPath}}
	if err := restoreDurableNonreplacementActivations(files); err != nil {
		t.Fatalf("restore durable activation: %v", err)
	}

	if got, err := os.ReadFile(destinationPath); err != nil || string(got) != "racer" {
		t.Fatalf("racing destination after restore = %q, %v", got, err)
	}
	if got, err := os.ReadFile(stagedPath); err != nil || string(got) != "uploaded" {
		t.Fatalf("staged upload after restore = %q, %v", got, err)
	}
	if _, err := os.Stat(stagedPath + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("activation marker remained after restore: %v", err)
	}
}

func TestRollbackDurableNonreplacementActivationsPreservesRacingDestination(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".stage", "photo.jpg")
	destinationPath := filepath.Join(root, "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("uploaded"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := activateDurableUploadDestination(stagedPath, destinationPath); err != nil {
		t.Fatalf("activate durable destination: %v", err)
	}
	if err := os.Remove(destinationPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destinationPath, []byte("racer"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: destinationPath}}
	if err := rollbackDurableNonreplacementActivations(files); err != nil {
		t.Fatalf("rollback durable activation: %v", err)
	}

	if got, err := os.ReadFile(destinationPath); err != nil || string(got) != "racer" {
		t.Fatalf("racing destination after rollback = %q, %v", got, err)
	}
	if _, err := os.Stat(stagedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged upload remained after rollback: %v", err)
	}
	if _, err := os.Stat(stagedPath + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("activation marker remained after rollback: %v", err)
	}
}
