/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"gooru.local/gooru/types"
	"github.com/spf13/cobra"
)

var (
	rehashShowProgress bool
)

// rehashCmd represents the rehash command
var rehashCmd = &cobra.Command{
	Use:   "rehash <file/dir/glob...>",
	Short: "Updates a file's content hash and preserves its tags.",
	Long: `Updates the database record for one or more files that have been modified on disk.

This command calculates the new content hash for a file and atomically transfers
all existing tags from the old content record to the new one. This is the primary
way to preserve a file's tagged identity after its content has changed.

This command only operates on paths that are already known to the database. It
will not add new, untracked files.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := expandFileArgs(args)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}
		if len(files) == 0 {
			fmt.Println("No matching files found to rehash.")
			return nil
		}

		processedCount := 0
		progressCb := func(path string, status types.RehashStatus, err error) {
			processedCount++
			if err != nil {
				fmt.Printf("Error rehashing '%s': %v\n", path, err)
				return
			}
			switch status {
			case types.StatusRehashed:
				fmt.Printf("Rehashed '%s'\n", path)
			case types.StatusSkippedUnchanged:
				if rehashShowProgress {
					fmt.Printf("Skipped '%s': file is already up-to-date.\n", path)
				}
			case types.StatusSkippedNotInDB:
				// This is a critical piece of user guidance. Always show it, regardless of progress flag.
				fmt.Printf("Skipped '%s': path not found in database. To update the location of a moved file, use: 'gooru editpath <oldpath> <newpath>'\n", path)
			case types.StatusMetadataUpdated:
				fmt.Printf("Updated metadata for '%s'\n", path)
			}
		}

		svc.RehashFiles(files, progressCb)

		if processedCount == 0 {
			fmt.Println("No files were processed.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(rehashCmd)
	rehashCmd.Flags().BoolVarP(&rehashShowProgress, "progress", "p", false, "Show detailed progress for all files, including skipped ones.")
}