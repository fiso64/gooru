package cmd

import (
    "fmt"
    "strings"

    "github.com/spf13/cobra"
    "gooru.local/internal/serve"
)

var importTargetID string

var importCmd = &cobra.Command{
    Use: "import <file/dir/glob...>",
    Short: "Copy local files into an upload target and register them",
    Long: "Import local files through the same staging and registration pipeline as an HTTP upload. Original source files are never moved or removed; --target is required.",
    Args: cobra.MinimumNArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        if strings.TrimSpace(importTargetID) == "" {
            return fmt.Errorf("--target is required; select a configured upload target ID")
        }
        expanded, err := expandFileArgs(args)
        if err != nil {
            return fmt.Errorf("expand import sources: %w", err)
        }
        if len(expanded.NotFound) != 0 {
            return fmt.Errorf("import sources not found: %s", strings.Join(expanded.NotFound, ", "))
        }
        if len(expanded.Found) == 0 {
            return fmt.Errorf("no matching import source files")
        }
        cfg, err := loadCommandConfig(serve.Overrides{DatabasePath: databasePath})
        if err != nil {
            return err
        }
        if err := ensureStorageEncryptionReady(cfg, svc); err != nil {
            return err
        }
        serverCfg := cfg
        if cfg.Encryption.Enabled {
            keys, err := configuredEncryptionKeys(cfg)
            if err != nil {
                return err
            }
            serverCfg.Encryption.Key = keys.Media
        }
        server := serve.NewServerWithLibrary(serverCfg, serve.NewGooruLibrary(svc, verbose))
        result, err := server.ImportLocalFiles(cmd.Context(), importTargetID, expanded.Found)
        if err != nil {
            return err
        }
        failed := 0
        for _, file := range result.Files {
            if file.Error != "" || file.Status == "error" {
                failed++
                fmt.Fprintf(cmd.OutOrStdout(), "%s: %s (%s)\n", file.Name, file.Status, file.Error)
                continue
            }
            fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", file.Name, file.Status)
        }
        if failed != 0 {
            return fmt.Errorf("%d local import files failed", failed)
        }
        return nil
    },
}

func init() {
    rootCmd.AddCommand(importCmd)
    importCmd.Flags().StringVar(&importTargetID, "target", "", "Required configured upload target ID")
}
