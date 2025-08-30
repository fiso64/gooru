/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	multiInputSetTags     bool
	setTagsShowProgress   bool
	setTagsExpressionMode bool
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
			files, err := expandFileArgs(fileSpecs)
			if err != nil {
				return fmt.Errorf("error expanding file arguments: %w", err)
			}

			if len(files) == 0 {
				fmt.Println("No files found to set tags on.")
				return nil
			}

			var filesFailed int
			progressCb := func(filePath string, err error) {
				if err != nil {
					filesFailed++
					fmt.Printf("Failed to set tags for '%s': %v\n", filePath, err)
				} else if setTagsShowProgress {
					if len(tags) > 0 {
						fmt.Printf("Set tags for '%s' to: %s\n", filePath, strings.Join(tags, ", "))
					} else {
						fmt.Printf("Removed all tags from '%s'\n", filePath)
					}
				}
			}

			if err := svc.SetTagsForFiles(files, tags, progressCb); err != nil {
				// This will be a DB error that caused a rollback.
				return fmt.Errorf("a database error occurred, all changes have been rolled back: %w", err)
			}

			if filesFailed > 0 {
				fmt.Printf("\nWarning: %d file(s) failed to process.\n", filesFailed)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(settagsCmd)
	settagsCmd.Flags().BoolVarP(&multiInputSetTags, "multi", "m", false, "Enable multi-input mode (for multiple file paths or expression parts)")
	settagsCmd.Flags().BoolVarP(&setTagsShowProgress, "progress", "p", false, "Show progress for each file processed")
	settagsCmd.Flags().BoolVarP(&setTagsExpressionMode, "expression", "e", false, "Use a query expression instead of file paths")
}
