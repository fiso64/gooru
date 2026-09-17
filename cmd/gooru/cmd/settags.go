/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/display"
	"gooru.local/types"
)

var (
	multiInputSetTags       bool
	setTagsExpressionMode   bool
	settagsUseMetadata      bool
	settagsRegistrationSort string
)

// settagsCmd represents the settags command
var settagsCmd = &cobra.Command{
	Use:   "settags <source> [tag1] [tag2...]",
	Short: "Sets the tags for files, replacing all existing ones.",
	Long: `Sets the tags for files, replacing any existing tags.
The source can be file paths, directories, glob patterns, or a query expression.
If no tags are provided, all existing tags will be removed from the matching files.

Default mode:
  gooru settags "archive/*.zip" archived

Multi-input mode (for complex file/expression lists):
  gooru settags -m /path/one.txt /path/two.png -- tag1 tag2
  gooru settags -e -m tag1 and tag2 -- newtag

Expression mode (set tags for files matching a query):
  gooru settags -e "project:alpha" archived version:1.0
  gooru settags -e "needs_review" # Removes all tags`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var fileSpecs []string
		var tags []string

		if multiInputSetTags {
			separatorIndex := cmd.Flags().ArgsLenAtDash()

			if separatorIndex == -1 {
				return errors.New("usage: gooru settags -m <file1>... -- [tag1]...\n(missing '--' separator)")
			}
			if separatorIndex == 0 {
				return errors.New("no files provided before '--' separator")
			}

			fileSpecs = args[:separatorIndex]
			tags = args[separatorIndex:]
		} else {
			if len(args) < 1 {
				return errors.New("usage: gooru settags <source> [tag1] [tag2...]")
			}
			fileSpecs = args[0:1]
			tags = args[1:]
		}

		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: settags\n")
			if setTagsExpressionMode {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed Expression: %v\n", fileSpecs)
			} else {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", fileSpecs)
			}
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed Tags: %v\n", tags)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		if setTagsExpressionMode {
			if len(fileSpecs) == 0 {
				return errors.New("expression mode (-e) requires an expression")
			}
			expression := strings.Join(fileSpecs, " ")
			affected, err := svc.SetTagsForFilesByQuery(expression, tags)
			if err != nil {
				return fmt.Errorf("a database error occurred: %w", err)
			}
			if len(tags) > 0 {
				fmt.Printf("Set tags for %d file(s) matching expression.\n", affected)
			} else {
				fmt.Printf("Removed all tags from %d file(s) matching expression.\n", affected)
			}
		} else {
			expansionResult, err := expandFileArgs(fileSpecs)
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
					fmt.Println("No files found to set tags on.")
				}
				return nil
			}

			var filesFailed, filesSucceeded int
			progressCb := func(filePath string, err error) {
				if err != nil {
					filesFailed++
					fmt.Printf("Failed to set tags for '%s': %v\n", filePath, err)
				} else {
					filesSucceeded++
				}
			}

			result, err := setTagsForFilesWithRegistrationSort(files, tags, progressCb, settagsUseMetadata, settagsRegistrationSort)
			if err != nil {
				// This will be a DB error that caused a rollback.
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
				if len(tags) > 0 {
					// The 'affected' count for settags (cleared+added) is not intuitive for users.
					// The simple message is clearer.
					fmt.Printf("Set tags for %d file(s).\n", filesSucceeded)
				} else {
					// This is for `settags file` (with no tags), which clears all tags.
					// Here, 'affected' is the number of cleared tags, which is useful.
					fmt.Printf("Removed %d tag association(s) from %d file(s).\n", result.AffectedCount, filesSucceeded)
				}
			}

			if filesFailed > 0 {
				fmt.Println() // For spacing
				display.Warnf("%d file(s) failed to process.", filesFailed)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(settagsCmd)
	settagsCmd.Flags().BoolVarP(&multiInputSetTags, "multi", "m", false, "Enable multi-input mode (for multiple file paths or expression parts)")
	settagsCmd.Flags().BoolVarP(&setTagsExpressionMode, "expression", "e", false, "Use a query expression instead of file paths")
	settagsCmd.Flags().BoolVar(&settagsUseMetadata, "use-metadata", false, "Use fast but unreliable (size+modtime) check to detect file changes")
	settagsCmd.Flags().StringVar(&settagsRegistrationSort, "sort", "newest-last", registrationSortHelp)
}
