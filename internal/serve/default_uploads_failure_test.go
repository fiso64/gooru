package serve

import (
 "os"
 "path/filepath"
 "testing"
)

func TestDefaultUploadDirectoryRejectsFile(t *testing.T) {
 p := filepath.Join(t.TempDir(), "uploads")
 if err := os.WriteFile(p, []byte("x"), 0600); err != nil { t.Fatal(err) }
 if err := prepareDefaultUploadDir(p); err == nil { t.Fatal("expected failure for file in place of directory") }
}
