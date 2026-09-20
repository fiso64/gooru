//go:build linux || darwin

package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultUploadDirectoryRejectsWritableParent(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "uploads")
	err := prepareDefaultUploadDir(target)
	if err == nil || !strings.Contains(err.Error(), "owner-controlled") {
		t.Fatalf("expected unsafe parent to be rejected: %v", err)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("unsafe parent received upload directory: %v", err)
	}
}
