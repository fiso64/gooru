package serve

import (
    "os"
    "path/filepath"
    "runtime"
    "testing"
)

func TestDefaultUploadDirectoryRejectsSymlinkedParent(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("symlink creation requires additional Windows privileges") }
    root := t.TempDir()
    realRoot := filepath.Join(root, "real")
    if err := os.Mkdir(realRoot, 0750); err != nil { t.Fatal(err) }
    link := filepath.Join(root, "link")
    if err := os.Symlink(realRoot, link); err != nil { t.Fatal(err) }
    err := prepareDefaultUploadDir(filepath.Join(link, "uploads"))
    if err == nil { t.Fatal("must reject a symlinked parent") }
    if _, err := os.Stat(filepath.Join(realRoot, "uploads")); !os.IsNotExist(err) {
        t.Fatalf("symlink destination was changed: %v", err)
    }
}

func TestDefaultUploadDirectoryRejectsSymlinkedTarget(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("symlink creation requires additional Windows privileges") }
    root := t.TempDir()
    realTarget := filepath.Join(root, "other")
    if err := os.Mkdir(realTarget, 0755); err != nil { t.Fatal(err) }
    path := filepath.Join(root, "uploads")
    if err := os.Symlink(realTarget, path); err != nil { t.Fatal(err) }
    if err := prepareDefaultUploadDir(path); err == nil { t.Fatal("must reject a symlinked upload target") }
    info, err := os.Stat(realTarget)
    if err != nil { t.Fatal(err) }
    if info.Mode().Perm() != 0755 { t.Fatalf("symlink destination permissions changed: %v", info.Mode()) }
}
