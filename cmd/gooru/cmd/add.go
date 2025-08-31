/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	addShowProgress bool
)

// addCmd represents the add command
var addCmd = &cobra.Command{
	Use:   "add <file/dir/glob...>",
	Short: "Adds or links files to be tracked by Gooru.",
	Long: `Adds or links one or more files to the Gooru database.

This is the primary command for tracking a file without immediately applying tags.
It also serves as a powerful tool to synchronize a specific file that has
been moved, renamed, or copied.

The 'add' command intelligently handles several scenarios:
- New files: Adds a new content record to the database.
- Moved/renamed files: Detects the move and updates the file's path.
- Duplicates: Links the new path as another location for existing content.

Note: You do not need to run 'add' before 'tag'. The 'tag' command
will automatically add and track any new files it's given.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := expandFileArgs(args)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}
		if len(files) == 0 {
			fmt.Println("No matching files found to add.")
			return nil
		}

		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: add\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", args)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		var filesFailed int
		progressCb := func(filePath string, err error) {
			if err != nil {
				filesFailed++
				fmt.Printf("Failed to process '%s': %v\n", filePath, err)
			} else if addShowProgress {
				// The service layer prints detailed move/modify messages.
				// This is just a simple confirmation for each file when -p is on.
				fmt.Printf("Processed '%s'\n", filePath)
			}
		}

		// `add` is just `tag` with no tags.
		if err := svc.TagFiles(files, []string{}, progressCb); err != nil {
			return fmt.Errorf("a database error occurred, all changes have been rolled back: %w", err)
		}

		if filesFailed > 0 {
			fmt.Printf("\nWarning: %d file(s) failed to process.\n", filesFailed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().BoolVarP(&addShowProgress, "progress", "p", false, "Show progress for each file processed")
}
