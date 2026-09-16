package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCommitUploadDestinationWithRenameRetryContinuesOriginalSuffixSequence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	initialPath, err := chooseUploadDestination(root, "photo.jpg", "rename")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(initialPath), "photo-1.jpg"; got != want {
		t.Fatalf("initial destination = %q, want %q", got, want)
	}
	stagedPath := filepath.Join(root, ".upload-staged")
	if err := os.WriteFile(stagedPath, []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initialPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}

	finalPath, err := commitUploadDestinationWithRenameRetry(stagedPath, root, "photo.jpg", initialPath, "rename")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(finalPath), "photo-2.jpg"; got != want {
		t.Fatalf("retry destination = %q, want %q", got, want)
	}
	if got := string(mustReadFile(t, initialPath)); got != "racer" {
		t.Fatalf("racing destination content = %q, want racer", got)
	}
	if got := string(mustReadFile(t, finalPath)); got != "staged" {
		t.Fatalf("retry destination content = %q, want staged", got)
	}
	if _, err := os.Stat(filepath.Join(root, "photo-1-1.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry nested the selected suffix instead of continuing the original sequence: %v", err)
	}
}

func TestCommitUploadDestinationWithRenameRetryDoesNotRetryErrorPolicy(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, ".upload-staged")
	if err := os.WriteFile(stagedPath, []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	destinationPath := filepath.Join(root, "photo.jpg")
	if err := os.WriteFile(destinationPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := commitUploadDestinationWithRenameRetry(stagedPath, root, "photo.jpg", destinationPath, "error"); !errors.Is(err, errUploadConflict) {
		t.Fatalf("commit error = %v, want upload conflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, "photo-1.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error policy unexpectedly allocated a rename destination: %v", err)
	}
}
