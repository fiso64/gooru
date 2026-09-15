package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveSavedUploadsRemovesOwnedDestination(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, ".gooru-upload-owned")
	if err := os.WriteFile(stagedPath, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}

	saved, err := finalizeStreamedUpload(UploadTarget{ID: "default", Path: dir}, streamedUpload{
		name: "owned.txt",
		path: stagedPath,
		size: int64(len("owned")),
	}, "rename", false)
	if err != nil {
		t.Fatalf("finalize upload: %v", err)
	}

	removeSavedUploads([]savedUpload{saved})
	if _, err := os.Stat(saved.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned destination still exists after rollback: %v", err)
	}
}

func TestRemoveSavedUploadsPreservesReplacementDestination(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, ".gooru-upload-owned")
	if err := os.WriteFile(stagedPath, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}

	saved, err := finalizeStreamedUpload(UploadTarget{ID: "default", Path: dir}, streamedUpload{
		name: "raced.txt",
		path: stagedPath,
		size: int64(len("owned")),
	}, "rename", false)
	if err != nil {
		t.Fatalf("finalize upload: %v", err)
	}
	if err := os.Remove(saved.path); err != nil {
		t.Fatalf("replace owned destination: %v", err)
	}
	if err := os.WriteFile(saved.path, []byte("replacement"), 0600); err != nil {
		t.Fatalf("write replacement destination: %v", err)
	}

	removeSavedUploads([]savedUpload{saved})
	got, err := os.ReadFile(saved.path)
	if err != nil {
		t.Fatalf("replacement destination was removed: %v", err)
	}
	if string(got) != "replacement" {
		t.Fatalf("replacement destination changed: %q", got)
	}
}
