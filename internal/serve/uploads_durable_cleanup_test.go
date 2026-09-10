package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveCanceledSavedUploadsReportsRetriableFailureWithoutPath(t *testing.T) {
	root := t.TempDir()
	stagedPath := filepath.Join(root, "blocked-stage")
	if err := os.Mkdir(stagedPath, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(stagedPath, "still-present")
	if err := os.WriteFile(child, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := removeCanceledSavedUploads([]savedUpload{{path: stagedPath}})
	if err == nil {
		t.Fatal("cleanup unexpectedly succeeded while staged path was non-empty")
	}
	if strings.Contains(err.Error(), stagedPath) {
		t.Fatalf("cleanup error leaked staged path: %v", err)
	}
	if _, statErr := os.Stat(stagedPath); statErr != nil {
		t.Fatalf("failed cleanup removed staged path unexpectedly: %v", statErr)
	}

	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := removeCanceledSavedUploads([]savedUpload{{path: stagedPath}}); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if _, err := os.Stat(stagedPath); !os.IsNotExist(err) {
		t.Fatalf("retry cleanup left staged path behind: %v", err)
	}
}
