/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/display"
	"gooru.local/types"
)

var (
	rehashUseMetadata bool
)

// rehashCmd represents the rehash command
var rehashCmd = &cobra.Command{
	Use:   "rehash <file/dir/glob...>",
	Short: "Updates a file's content hash and preserves its tags.",
	Long: `Updates the database record for one or more files that have been modified on disk.

This command calculates the new content hash for a file and atomically preserves
its existing tags on the new content record. This is the primary way to preserve
a file's tagged identity after its content has changed.

This command only operates on paths that are already known to the database. It
will not add new, untracked files.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		expansionResult, err := expandFileArgs(args)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		// Managed WebUI uploads may have a canonical logical path that is not a
		// filesystem path. Keep unmatched literal arguments so the service can
		// resolve them through the database's managed-storage mapping. Ordinary
		// typos remain harmless: the service reports them as not tracked.
		files := append([]string{}, expansionResult.Found...)
		files = append(files, expansionResult.NotFound...)
		if len(files) == 0 {
			fmt.Println("No matching files found to rehash.")
			return nil
		}

		var rehashedCount, updatedCount, errorCount int

		progressCb := func(path string, status types.RehashStatus, err error) {
			if err != nil {
				errorCount++
				display.Errorf("rehashing '%s': %v", path, err)
				return
			}
			switch status {
			case types.StatusRehashed:
				rehashedCount++
			case types.StatusSkippedNotInDB:
				fmt.Printf("Skipped '%s': path not found in database. To update the location of a moved file, use: 'gooru editpath <oldpath> <newpath>'\n", path)
			case types.StatusMetadataUpdated:
				updatedCount++
			}
		}

		svc.RehashTrackedFiles(files, progressCb, rehashUseMetadata)

		if rehashedCount > 0 || updatedCount > 0 {
			fmt.Printf("Rehash complete. %d file(s) rehashed, %d metadata record(s) updated.\n", rehashedCount, updatedCount)
		} else if len(files) > 0 && errorCount == 0 {
			fmt.Println("Rehash complete. No files required updates.")
		}

		if errorCount > 0 {
			display.Warnf("%d error(s) occurred during rehash.", errorCount)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(rehashCmd)
	rehashCmd.Flags().BoolVar(&rehashUseMetadata, "use-metadata", false, "Use fast but unreliable (size+modtime) check to detect file changes")
}
