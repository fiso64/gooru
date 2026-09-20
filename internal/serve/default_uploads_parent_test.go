package serve

import (
    "os"
    "path/filepath"
    "testing"
)

func TestDefaultUploadDirectoryDoesNotChmodParent(t *testing.T) {
    root := t.TempDir()
    if err := os.Chmod(root, 0750); err != nil { t.Fatal(err) }
    if err := prepareDefaultUploadDir(filepath.Join(root, "uploads")); err != nil { t.Fatal(err) }
    info, err := os.Stat(root)
    if err != nil { t.Fatal(err) }
    if info.Mode().Perm() != 0750 { t.Fatalf("existing parent permissions changed: %v", info.Mode()) }
}
