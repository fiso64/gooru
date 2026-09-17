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
	multiInputTag       bool
	tagExpressionMode   bool
	tagUseMetadata      bool
	tagRegistrationSort string
)

// tagCmd represents the tag command
var tagCmd = &cobra.Command{
	Use:   "tag <source> <tag1> [tag2...]",
	Short: "Tags files with one or more tags.",
	Long: `Tags files with the given tags.
The source can be file paths, directories, glob patterns, or a query expression.

Default mode:
  gooru tag <file/dir/glob> <tag1> [tag2...]
  Example: gooru tag "photos/*.jpg" vacation summer

Multi-input mode (for complex file/expression lists):
  gooru tag -m /path/one.txt /path/two.png -- tag1 tag2
  gooru tag -e -m tag1 and tag2 -- newtag

Expression mode (tag files matching a query):
  gooru tag -e "photo -vacation" needs_review`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var fileSpecs []string
		var tags []string

		if multiInputTag {
			separatorIndex := cmd.Flags().ArgsLenAtDash()

			if separatorIndex == -1 {
				return errors.New("usage: gooru tag -m <file1>... -- <tag1>...\n(missing '--' separator)")
			}
			if separatorIndex == 0 {
				return errors.New("no files provided before '--' separator")
			}
			if separatorIndex == len(args) {
				return errors.New("no tags provided after '--' separator. To add files without tags, use the 'add' command")
			}

			fileSpecs = args[:separatorIndex]
			tags = args[separatorIndex:]
		} else {
			if len(args) < 2 {
				return errors.New("usage: gooru tag <source> <tag1> [tag2...]. To add files without tags, use the 'add' command")
			}
			fileSpecs = args[0:1]
			tags = args[1:]
		}

		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: tag\n")
			if tagExpressionMode {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed Expression: %v\n", fileSpecs)
			} else {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", fileSpecs)
			}
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed Tags: %v\n", tags)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		if tagExpressionMode {
			if len(fileSpecs) == 0 {
				return errors.New("expression mode (-e) requires an expression")
			}
			expression := strings.Join(fileSpecs, " ")
			affected, err := svc.TagFilesByQuery(expression, tags)
			if err != nil {
				return fmt.Errorf("a database error occurred: %w", err)
			}
			fmt.Printf("Added %d tag associations matching expression.\n", affected)
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
					fmt.Println("No files found to tag.")
				}
				return nil
			}

			var filesFailed, filesSucceeded int
			progressCb := func(filePath string, err error) {
				if err != nil {
					filesFailed++
					fmt.Printf("Failed to tag '%s': %v\n", filePath, err)
				} else {
					filesSucceeded++
				}
			}

			result, err := tagFilesWithRegistrationSort(files, tags, progressCb, tagUseMetadata, tagRegistrationSort)
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
				if result.AffectedCount > 0 {
					fmt.Printf("Added %d new tag association(s) to %d file(s).\n", result.AffectedCount, filesSucceeded)
				} else {
					fmt.Printf("Processed %d file(s); all specified tags were already present.\n", filesSucceeded)
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
	rootCmd.AddCommand(tagCmd)
	tagCmd.Flags().BoolVarP(&multiInputTag, "multi", "m", false, "Enable multi-input mode (for multiple file paths or expression parts)")
	tagCmd.Flags().BoolVarP(&tagExpressionMode, "expression", "e", false, "Use a query expression instead of file paths")
	tagCmd.Flags().BoolVar(&tagUseMetadata, "use-metadata", false, "Use fast but unreliable (size+modtime) check to detect file changes")
	tagCmd.Flags().StringVar(&tagRegistrationSort, "sort", "queue", registrationSortHelp)
}
