package serve

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "testing"

    core "gooru.local/gooru"
    "gooru.local/internal/securekey"
    "gooru.local/types"
)

func TestLocalImportPreservesSourceAndProtectsManagedDestination(t *testing.T) {
    dir := t.TempDir()
    dbPath := filepath.Join(dir, "gooru.db")
    writeInitializedTestDB(t, dbPath, types.StrategyFull)
    client, err := core.New(dbPath, false)
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = client.Close() })
    source := filepath.Join(dir, "source", "secret.txt")
    if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil { t.Fatal(err) }
    plaintext := []byte("local import is not plaintext in managed storage")
    if err := os.WriteFile(source, plaintext, 0600); err != nil { t.Fatal(err) }
    target := filepath.Join(dir, "upload-target")
    cfg := DefaultConfig(filepath.Join(dir, "serve.db"))
    cfg.Auth.Enabled = false
    cfg.Encryption.Enabled = true
    cfg.Encryption.Key = bytes.Repeat([]byte{0x62}, securekey.Size)
    cfg.Uploads.Enabled = true
    cfg.Uploads.Targets = []UploadTarget{{ID: "private", Name: "Private", Path: target}}
    server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))
    result, err := server.ImportLocalFiles(context.Background(), "private", []string{source})
    if err != nil { t.Fatal(err) }
    if len(result.Files) != 1 || result.Files[0].Status != "imported" { t.Fatalf("import response: %+v", result) }
    if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, plaintext) { t.Fatalf("source lost or modified: %v", err) }
    logical := filepath.Join(target, "secret.txt")
    if _, err := os.Stat(logical); !os.IsNotExist(err) { t.Fatalf("protected logical file still exists: %v", err) }
    file, err := client.GetFileInfoByPath(logical)
    if err != nil { t.Fatal(err) }
    file, err = client.ResolveManagedStorage(file)
    if err != nil { t.Fatal(err) }
    if file.StoragePath == "" { t.Fatal("managed storage path missing") }
    ciphertext, err := os.ReadFile(file.StoragePath)
    if err != nil { t.Fatal(err) }
    if bytes.Contains(ciphertext, plaintext) { t.Fatal("managed target persisted plaintext") }
}
