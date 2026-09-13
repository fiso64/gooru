package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveCanceledSavedUploadsPrunesOnlyEmptyRecognizedStagingDirs(t *testing.T) {
	root := t.TempDir()
	stagingRoot := filepath.Join(root, durableUploadStagingRootName)
	operationDir := filepath.Join(stagingRoot, "operation-prune-test")
	firstRequestDir := filepath.Join(operationDir, "request-first")
	secondRequestDir := filepath.Join(operationDir, "request-second")
	if err := os.MkdirAll(firstRequestDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondRequestDir, 0700); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(firstRequestDir, "first.jpg")
	secondPath := filepath.Join(secondRequestDir, "second.jpg")
	if err := os.WriteFile(firstPath, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := removeCanceledSavedUploads([]savedUpload{{path: firstPath}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstRequestDir); !os.IsNotExist(err) {
		t.Fatalf("empty request staging dir should be pruned: %v", err)
	}
	if got := string(mustReadFile(t, secondPath)); got != "second" {
		t.Fatalf("sibling staged content = %q, want second", got)
	}
	if _, err := os.Stat(operationDir); err != nil {
		t.Fatalf("operation staging dir with sibling request should remain: %v", err)
	}

	if err := removeCanceledSavedUploads([]savedUpload{{path: secondPath}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagingRoot); !os.IsNotExist(err) {
		t.Fatalf("empty staging hierarchy should be pruned: %v", err)
	}
}

func TestRemoveCanceledSavedUploadsPrunesLegacyOperationStaging(t *testing.T) {
	root := t.TempDir()
	stagingRoot := filepath.Join(root, durableUploadStagingRootName)
	operationDir := filepath.Join(stagingRoot, "operation-legacy-test")
	if err := os.MkdirAll(operationDir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(operationDir, "legacy.jpg")
	if err := os.WriteFile(legacyPath, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := removeCanceledSavedUploads([]savedUpload{{path: legacyPath}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagingRoot); !os.IsNotExist(err) {
		t.Fatalf("legacy staging hierarchy should be pruned: %v", err)
	}
}
