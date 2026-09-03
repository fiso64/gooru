package serve

import (
	"os"
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

func TestIsManagedUploadPathRejectsNestedSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "uploads")
	external := filepath.Join(dir, "external")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	path := filepath.Join(link, "outside.jpg")
	if err := os.WriteFile(filepath.Join(external, "outside.jpg"), []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsManagedUploadPath([]UploadTarget{{ID: "managed", Name: "Managed", Path: root}}, path) {
		t.Fatal("path escaping through a nested symlink must not be managed")
	}
}

func TestIsManagedUploadPathAllowsSymlinkedRoot(t *testing.T) {
	dir := t.TempDir()
	realRoot := filepath.Join(dir, "real-uploads")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(dir, "uploads")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	path := filepath.Join(linkedRoot, "nested", "future.jpg")

	if !IsManagedUploadPath([]UploadTarget{{ID: "managed", Name: "Managed", Path: linkedRoot}}, path) {
		t.Fatal("symlinked upload root should recognize files beneath its resolved target")
	}
}
