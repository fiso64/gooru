package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackDurableReplacementPreservesRacingDestination(t *testing.T) {
	root := t.TempDir()
	stagingDir := filepath.Join(root, durableUploadStagingRootName, testDurableUploadOperationID)
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		t.Fatal(err)
	}
	stagedPath := filepath.Join(stagingDir, "photo.jpg")
	finalPath := filepath.Join(root, "photo.jpg")
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{
		name:            "photo.jpg",
		path:            stagedPath,
		destinationPath: finalPath,
		targetID:        "default",
		replace:         true,
	}}
	if err := prepareDurableReplacementRecoveryMarkers(files); err != nil {
		t.Fatal(err)
	}
	activated, err := activateSavedDurableReplacements(files)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "replacement" {
		t.Fatalf("activated destination = %q, want replacement", got)
	}

	if err := os.Remove(finalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := rollbackDurableSavedReplacements(files, activated); !errors.Is(err, errUploadConflict) {
		t.Fatalf("rollback error = %v, want upload conflict", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "racer" {
		t.Fatalf("rollback changed racing destination to %q", got)
	}
	if got := string(mustReadFile(t, activated[0].replacement.backupPath)); got != "original" {
		t.Fatalf("rollback changed preserved original to %q", got)
	}
	if got := string(mustReadFile(t, stagedPath+durableUploadActivatedMarkerSuffix)); got != "replacement" {
		t.Fatalf("rollback changed recovery marker to %q", got)
	}
}

func TestRollbackDurableReplacementWithoutMarkerPreservesExistingDestination(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".photo.jpg.tmp")
	finalPath := filepath.Join(root, "photo.jpg")
	backupPath := stagedPath + ".backup"
	if err := os.WriteFile(finalPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	file := savedUpload{path: stagedPath, destinationPath: finalPath, replace: true}
	replacement := activatedReplacement{finalPath: finalPath, backupPath: backupPath, hadOriginal: true}

	if err := rollbackDurableReplacement(file, replacement); !errors.Is(err, errUploadConflict) {
		t.Fatalf("rollback error = %v, want upload conflict", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "racer" {
		t.Fatalf("rollback changed unowned destination to %q", got)
	}
	if got := string(mustReadFile(t, backupPath)); got != "original" {
		t.Fatalf("rollback changed preserved original to %q", got)
	}
}

func TestRestoreActivatedReplacementDestinationsRejectsMarkerlessDestination(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".photo.jpg.tmp")
	finalPath := filepath.Join(root, "photo.jpg")
	if err := os.WriteFile(finalPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{path: stagedPath, destinationPath: finalPath, replace: true}}
	activated := []activatedSavedReplacement{{
		index:       0,
		replacement: activatedReplacement{finalPath: finalPath},
	}}

	if err := restoreActivatedReplacementDestinations(files, activated); !errors.Is(err, errUploadConflict) {
		t.Fatalf("restore error = %v, want upload conflict", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "racer" {
		t.Fatalf("restore changed markerless destination to %q", got)
	}
}

func TestRollbackDurableReplacementRestoresOriginal(t *testing.T) {
	root := t.TempDir()
	stagingDir := filepath.Join(root, durableUploadStagingRootName, testDurableUploadOperationID)
	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		t.Fatal(err)
	}
	stagedPath := filepath.Join(stagingDir, "photo.jpg")
	finalPath := filepath.Join(root, "photo.jpg")
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{
		name:            "photo.jpg",
		path:            stagedPath,
		destinationPath: finalPath,
		targetID:        "default",
		replace:         true,
	}}
	if err := prepareDurableReplacementRecoveryMarkers(files); err != nil {
		t.Fatal(err)
	}
	activated, err := activateSavedDurableReplacements(files)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollbackDurableSavedReplacements(files, activated); err != nil {
		t.Fatal(err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "original" {
		t.Fatalf("rollback destination = %q, want original", got)
	}
	if _, err := os.Stat(activated[0].replacement.backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rollback backup still exists: %v", err)
	}
	if err := settleDurableReplacementRecoveryMarkers(files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagedPath + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery marker still exists after settlement: %v", err)
	}
}
