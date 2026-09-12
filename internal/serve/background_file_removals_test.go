package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistedStagedFileDeletionRejectsUnrelatedPath(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "file.jpg")
	staged := filepath.Join(t.TempDir(), ".gooru-delete-token", "file.jpg")
	if _, err := persistedStagedFileDeletion(original, staged); err == nil {
		t.Fatal("expected unrelated staging path to be rejected")
	}
}

func TestPersistedStagedFileDeletionCanCommitAfterOriginalIsGone(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "file.jpg")
	if err := os.WriteFile(original, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	stagedPath := filepath.Join(root, ".gooru-delete-token", "file.jpg")
	staged, err := persistedStagedFileDeletion(original, stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.stage(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("original should be staged, stat err=%v", err)
	}
	if err := staged.commitMissingOK(); err != nil {
		t.Fatal(err)
	}
	if err := staged.commitMissingOK(); err != nil {
		t.Fatalf("commit must be idempotent: %v", err)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("staged file should be gone, stat err=%v", err)
	}
}

func TestPersistedStagedFileDeletionRollbackDoesNotOverwriteReplacement(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "file.jpg")
	if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	stagedPath := filepath.Join(root, ".gooru-delete-token", "file.jpg")
	staged, err := persistedStagedFileDeletion(original, stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.stage(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := staged.rollbackMissingOK(); err == nil {
		t.Fatal("expected rollback to reject occupied original path")
	}

	got, err := os.ReadFile(original)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replacement" {
		t.Fatalf("replacement was modified: got %q", got)
	}
	stagedData, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("staged original should remain recoverable: %v", err)
	}
	if string(stagedData) != "original" {
		t.Fatalf("staged original was modified: got %q", stagedData)
	}
}
