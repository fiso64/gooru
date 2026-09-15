package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRollbackReplacementPreservesRacingDestination(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "staged")
	finalPath := filepath.Join(dir, "final")
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(finalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := rollbackReplacement(replacement); !errors.Is(err, errUploadConflict) {
		t.Fatalf("rollback error = %v, want upload conflict", err)
	}
	assertUploadFileContents(t, finalPath, "racer")
	assertUploadFileContents(t, replacement.backupPath, "original")
}

func TestRollbackReplacementWithoutOriginalPreservesRacingDestination(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "staged")
	finalPath := filepath.Join(dir, "final")
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(finalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := rollbackReplacement(replacement); !errors.Is(err, errUploadConflict) {
		t.Fatalf("rollback error = %v, want upload conflict", err)
	}
	assertUploadFileContents(t, finalPath, "racer")
	if _, err := os.Stat(replacement.noOriginalMarkerPath); err != nil {
		t.Fatalf("no-original marker should remain for recovery: %v", err)
	}
}

func TestRollbackReplacementRestoresOriginalWithOwnershipMarker(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "staged")
	finalPath := filepath.Join(dir, "final")
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollbackReplacement(replacement); err != nil {
		t.Fatal(err)
	}

	assertUploadFileContents(t, finalPath, "original")
	if _, err := os.Stat(replacement.ownershipMarkerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ownership marker still exists after rollback: %v", err)
	}
}

func TestRollbackReplacementWithoutOriginalClearsOwnershipMarker(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, "staged")
	finalPath := filepath.Join(dir, "final")
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := rollbackReplacement(replacement); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(finalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement destination still exists after rollback: %v", err)
	}
	if _, err := os.Stat(replacement.noOriginalMarkerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no-original marker still exists after rollback: %v", err)
	}
	if _, err := os.Stat(replacement.ownershipMarkerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ownership marker still exists after rollback: %v", err)
	}
}

func assertUploadFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s contents = %q, want %q", path, got, want)
	}
}
