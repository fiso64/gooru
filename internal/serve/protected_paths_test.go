package serve

import (
	"path/filepath"
	"testing"
)

func TestIsManagedUploadPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "uploads")
	targets := []UploadTarget{{ID: "managed", Name: "Managed", Path: root}}

	if !IsManagedUploadPath(targets, filepath.Join(root, "nested", "file.jpg")) {
		t.Fatal("nested upload path should be managed")
	}
	if IsManagedUploadPath(targets, root) {
		t.Fatal("upload root itself is not a managed file path")
	}
	if IsManagedUploadPath(targets, root+"-other/file.jpg") {
		t.Fatal("sibling prefix must not be treated as managed")
	}
	if IsManagedUploadPath(targets, filepath.Join(filepath.Dir(root), "external", "file.jpg")) {
		t.Fatal("external path must not be treated as managed")
	}
}
