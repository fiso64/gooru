/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/display"
	"gooru.local/types"
)

var (
	addUseMetadata      bool
	addRegistrationSort string
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
		expansionResult, err := expandFileArgs(args)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		for _, notFound := range expansionResult.NotFound {
			display.Warnf("skipping path not found: %s", notFound)
		}

		files := expansionResult.Found
		if len(files) == 0 {
			// Only print if there were no "not found" warnings, to avoid redundancy.
			if len(expansionResult.NotFound) == 0 {
				fmt.Println("No matching files found to add.")
			}
			return nil
		}

		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: add\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", args)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		var filesFailed, filesSucceeded int
		progressCb := func(filePath string, err error) {
			if err != nil {
				filesFailed++
				fmt.Printf("Failed to process '%s': %v\n", filePath, err)
			} else {
				filesSucceeded++
			}
		}

		// `add` is just `tag` with no tags.
		result, err := tagFilesWithRegistrationSort(files, []string{}, progressCb, addUseMetadata, addRegistrationSort)
		if err != nil {
			return fmt.Errorf("a database error occurred, all changes have been rolled back: %w", err)
		}

		for _, n := range result.Notifications {
			switch n.Kind {
			case types.NotificationKindModified:
				fmt.Printf("Updated database for modified file: '%s'\n", n.OriginalPath)
				if len(n.OrphanedTags) > 0 {
					display.Warnf("'%s' was modified. The old version's tags [%s] are now orphaned. Run 'gooru relinkall' to find moved copies or 'gooru prune' to clean up.", n.OriginalPath, strings.Join(n.OrphanedTags, ", "))
				}
			case types.NotificationKindMoveDetected:
				fmt.Printf("Detected move for known content: '%s' -> '%s'\n", n.OldPath, n.NewPath)
			}
		}

		if filesSucceeded > 0 {
			fmt.Printf("Processed %d file(s).\n", filesSucceeded)
		}

		if filesFailed > 0 {
			fmt.Println() // For spacing
			display.Warnf("%d file(s) failed to process.", filesFailed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().BoolVar(&addUseMetadata, "use-metadata", false, "Use fast but unreliable (size+modtime) check to detect file changes")
	addCmd.Flags().StringVar(&addRegistrationSort, "sort", "newest-last", registrationSortHelp)
}
