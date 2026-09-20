package serve

import (
    "path/filepath"
    "runtime"
    "testing"
)

func TestManagedDefaultUploadPathMatchesService(t *testing.T) {
    if runtime.GOOS != "linux" { t.Skip("Linux only") }
    config := "/etc/gooru/main/serve.yaml"
    t.Setenv("STATE_DIRECTORY", "")
    cliPath, err := defaultUploadPathForConfig(config)
    if err != nil { t.Fatal(err) }
    want := filepath.Join("/var/lib", "gooru-main", "uploads")
    if cliPath != want { t.Fatalf("CLI: %q, want %q", cliPath, want) }
    t.Setenv("STATE_DIRECTORY", filepath.Join(t.TempDir(), "unrelated"))
    servicePath, err := defaultUploadPathForConfig(config)
    if err != nil { t.Fatal(err) }
    if servicePath != cliPath { t.Fatalf("service: %q; CLI: %q", servicePath, cliPath) }
}
