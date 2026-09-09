package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscardDuplicateUploadPreservesTrackedDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tracked.jpg")
	if err := os.WriteFile(path, []byte("tracked"), 0600); err != nil {
		t.Fatalf("write tracked destination: %v", err)
	}

	discardDuplicateUpload(StagedUpload{Path: path, AnalysisPath: path}, true)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("tracked destination was removed: %v", err)
	}
	if got := string(data); got != "tracked" {
		t.Fatalf("tracked destination = %q, want tracked", got)
	}
}

func TestDiscardDuplicateUploadRemovesRejectedStaging(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "staged.jpg")
	analysisPath := filepath.Join(dir, "analysis.jpg")
	for _, item := range []string{path, analysisPath} {
		if err := os.WriteFile(item, []byte("staged"), 0600); err != nil {
			t.Fatalf("write %s: %v", item, err)
		}
	}

	discardDuplicateUpload(StagedUpload{Path: path, AnalysisPath: analysisPath}, false)

	for _, item := range []string{path, analysisPath} {
		if _, err := os.Stat(item); !os.IsNotExist(err) {
			t.Fatalf("%s stat = %v, want not exist", item, err)
		}
	}
}
