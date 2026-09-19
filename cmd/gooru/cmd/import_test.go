package cmd

import (
    "bytes"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "gooru.local/gooru"
    "gooru.local/types"
)

// Exercise the real Cobra entry point, database registration and configured upload
// target together; serve-level importer tests alone cannot catch CLI wiring bugs.
func TestImportCommandCopiesAndRegistersToConfiguredTarget(t *testing.T) {
    root := t.TempDir()
    database := filepath.Join(root, "gooru.db")
    if err := gooru.Init(database, types.StrategyFull, false); err != nil {
        t.Fatal(err)
    }
    source := filepath.Join(root, "source.txt")
    if err := os.WriteFile(source, []byte("CLI import golden path"), 0600); err != nil {
        t.Fatal(err)
    }
    target := filepath.Join(root, "target")
    configFile := filepath.Join(root, "config.yaml")
    configuration := fmt.Sprintf("auth:\n  enabled: false\nuploads:\n  enabled: true\n  targets:\n    - id: archive\n      name: Archive\n      path: %q\n", target)
    if err := os.WriteFile(configFile, []byte(configuration), 0600); err != nil {
        t.Fatal(err)
    }

    var output bytes.Buffer
    previousDatabase, previousConfig, previousTarget := databasePath, configPath, importTargetID
    t.Cleanup(func() {
        rootCmd.SetArgs(nil)
        rootCmd.SetOut(nil)
        databasePath, configPath, importTargetID = previousDatabase, previousConfig, previousTarget
        if svc != nil {
            _ = svc.Close()
            svc = nil
        }
    })
    rootCmd.SetOut(&output)
    rootCmd.SetArgs([]string{"--database", database, "--config", configFile, "import", "--target", "archive", source})
    if err := rootCmd.Execute(); err != nil {
        t.Fatalf("CLI import failed: %v", err)
    }
    if !strings.Contains(output.String(), "source.txt: imported") {
        t.Fatalf("missing CLI success result: %q", output.String())
    }
    if got, err := os.ReadFile(source); err != nil || string(got) != "CLI import golden path" {
        t.Fatalf("original source changed: %q, %v", got, err)
    }
    imported := filepath.Join(target, "source.txt")
    if got, err := os.ReadFile(imported); err != nil || string(got) != "CLI import golden path" {
        t.Fatalf("target copy missing: %q, %v", got, err)
    }
    client, err := gooru.New(database, false)
    if err != nil {
        t.Fatal(err)
    }
    defer client.Close()
    if _, err := client.GetFileInfoByPath(imported); err != nil {
        t.Fatalf("imported destination was not registered: %v", err)
    }
}
