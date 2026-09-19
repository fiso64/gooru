package serve

import (
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"
)

func TestImportLocalFilesCopiesToExplicitTargetWithoutMovingSources(t *testing.T) {
    root := t.TempDir()
    sourceDir := filepath.Join(root, "source")
    if err := os.MkdirAll(sourceDir, 0700); err != nil { t.Fatal(err) }
    source := filepath.Join(sourceDir, "photo.jpg")
    if err := os.WriteFile(source, []byte("local file bytes"), 0600); err != nil { t.Fatal(err) }
    stamp := time.Date(2020, time.July, 8, 9, 10, 11, 0, time.UTC)
    if err := os.Chtimes(source, stamp, stamp); err != nil { t.Fatal(err) }

    defaultDir := filepath.Join(root, "default")
    archiveDir := filepath.Join(root, "archive")
    library := &recordingUploadLibrary{}
    server := newUploadTestServer(t, defaultDir, true, library)
    server.cfg.Uploads.Targets = append(server.cfg.Uploads.Targets, UploadTarget{ID: "archive", Name: "Archive", Path: archiveDir})
    server.cfg.Uploads.PreserveModTime = true
    response, err := server.ImportLocalFiles(context.Background(), "archive", []string{source})
    if err != nil { t.Fatal(err) }
    if len(response.Files) != 1 || response.Files[0].TargetID != "archive" { t.Fatalf("response: %+v", response) }
    if got, err := os.ReadFile(source); err != nil || string(got) != "local file bytes" { t.Fatalf("source changed: %q, %v", got, err) }
    if got, err := os.ReadFile(filepath.Join(archiveDir, "photo.jpg")); err != nil || string(got) != "local file bytes" { t.Fatalf("target content: %q, %v", got, err) }
    if _, err := os.Stat(filepath.Join(defaultDir, "photo.jpg")); !os.IsNotExist(err) { t.Fatalf("unexpected default-target copy: %v", err) }
    if len(library.files) != 1 || library.files[0].TargetID != "archive" || !library.files[0].SourceModTime.Equal(stamp) {
        t.Fatalf("importer did not receive requested target/modtime: %+v", library.files)
    }
}

func TestImportLocalFilesRejectsTargetAndNonregularSourceBeforeCopy(t *testing.T) {
    root := t.TempDir()
    target := filepath.Join(root, "target")
    source := filepath.Join(root, "source.txt")
    if err := os.WriteFile(source, []byte("unchanged"), 0600); err != nil { t.Fatal(err) }
    library := &recordingUploadLibrary{}
    server := newUploadTestServer(t, target, true, library)
    for _, id := range []string{"", "missing"} {
        if _, err := server.ImportLocalFiles(context.Background(), id, []string{source}); err == nil {
            t.Fatalf("invalid target %q unexpectedly accepted", id)
        }
    }
    symlink := filepath.Join(root, "linked.txt")
    if err := os.Symlink(source, symlink); err != nil { t.Fatal(err) }
    if _, err := server.ImportLocalFiles(context.Background(), "default", []string{symlink}); err == nil || !strings.Contains(err.Error(), "not a regular file") {
        t.Fatalf("symlink source error: %v", err)
    }
    if got, err := os.ReadFile(source); err != nil || string(got) != "unchanged" { t.Fatalf("source changed: %q, %v", got, err) }
    if _, err := os.Stat(filepath.Join(target, "linked.txt")); !os.IsNotExist(err) { t.Fatalf("failed import left copied file: %v", err) }
}

func TestImportLocalFilesPreservesExistingNameViaUploadConflictPolicy(t *testing.T) {
    root := t.TempDir()
    source := filepath.Join(root, "source", "same.txt")
    target := filepath.Join(root, "target")
    if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil { t.Fatal(err) }
    if err := os.MkdirAll(target, 0700); err != nil { t.Fatal(err) }
    if err := os.WriteFile(source, []byte("new"), 0600); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(target, "same.txt"), []byte("old"), 0600); err != nil { t.Fatal(err) }
    server := newUploadTestServer(t, target, true, &recordingUploadLibrary{})
    if _, err := server.ImportLocalFiles(context.Background(), "default", []string{source}); err != nil { t.Fatal(err) }
    old, _ := os.ReadFile(filepath.Join(target, "same.txt"))
    renamed, renameErr := os.ReadFile(filepath.Join(target, "same-1.txt"))
    if string(old) != "old" || renameErr != nil || string(renamed) != "new" { t.Fatalf("conflict overwrote source/destination: old=%q, renamed=%q, err=%v", old, renamed, renameErr) }
}
