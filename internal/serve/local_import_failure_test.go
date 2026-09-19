package serve

import (
    "bytes"
    "context"
    "errors"
    "os"
    "path/filepath"
    "testing"

    core "gooru.local/gooru"
    "gooru.local/internal/securekey"
    "gooru.local/types"
)

func TestLocalImportProtectedPartialSourceFailure(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "gooru.db")
    writeInitializedTestDB(t, dbPath, types.StrategyFull)
    client, err := core.New(dbPath, false)
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = client.Close() })
    source := filepath.Join(dir, "source", "first.txt")
    if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil { t.Fatal(err) }
    plaintext := []byte("abort partial protected local import before durable task")
    if err := os.WriteFile(source, plaintext, 0600); err != nil { t.Fatal(err) }
    symlink := filepath.Join(dir, "source", "second.txt")
    if err := os.Symlink(source, symlink); err != nil { t.Fatal(err) }
    target := filepath.Join(dir, "upload-target")
    cfg := DefaultConfig(filepath.Join(dir, "serve.db"))
    cfg.Auth.Enabled = false
    cfg.Encryption.Enabled = true
    cfg.Encryption.Key = bytes.Repeat([]byte{0x62}, securekey.Size)
    cfg.Uploads.Enabled = true
    cfg.Uploads.Targets = []UploadTarget{{ID: "private", Name: "Private", Path: target}}
    server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))
    if _, err := server.ImportLocalFiles(context.Background(), "private", []string{source, symlink}); err == nil {
        t.Fatal("expected partial source failure")
    }
    if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, plaintext) {
        t.Fatalf("original source changed: %q, %v", got, err)
    }
    if _, err := os.Stat(filepath.Join(target, "first.txt")); !errors.Is(err, os.ErrNotExist) {
        t.Fatalf("failed protected import left a logical file: %v", err)
    }
    if _, err := client.GetFileInfoByPath(filepath.Join(target, "first.txt")); err == nil {
        t.Fatal("failed protected import registered the partial source")
    }
    if err := filepath.WalkDir(target, func(path string, entry os.DirEntry, walkErr error) error {
        if errors.Is(walkErr, os.ErrNotExist) { return nil }
        if walkErr != nil { return walkErr }
        if entry.IsDir() { return nil }
        contents, err := os.ReadFile(path)
        if err != nil { return err }
        if bytes.Contains(contents, plaintext) { t.Errorf("failed import left plaintext at %q", path) }
        return nil
    }); err != nil {
        t.Fatal(err)
    }
}
