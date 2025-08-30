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
	multiInputUntag     bool
	untagShowProgress   bool
	untagExpressionMode bool
)

// untagCmd represents the untag command
var untagCmd = &cobra.Command{
	Use:   "untag <source> [tag1] [tag2...]",
	Short: "Removes tags from files. If no tags are given, all tags are removed.",
	Long: `Removes one or more tags from files.
The source can be file paths, directories, glob patterns, or a query expression.
If no tags are provided, it removes ALL tags from the matching files.

Default mode:
  gooru untag "tmp/*" temporary
  gooru untag "archive.zip"  # Removes all tags from archive.zip

Multi-input mode (for complex file/expression lists):
  gooru untag -m file1.txt file2.png -- tag1 tag2
  gooru untag -e -m tag1 and tag2 -- oldtag

Expression mode (untag files matching a query):
  gooru untag -e "temporary" temporary
  gooru untag -e "archived" # Removes all tags`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var fileSpecs []string
		var tags []string

		if multiInputUntag {
			separatorIndex := cmd.Flags().ArgsLenAtDash()

			if separatorIndex == -1 {
				return errors.New("usage: gooru untag -m <file1>... -- [tag1]...\n(missing '--' separator)")
			}
			if separatorIndex == 0 {
				return errors.New("no files provided before '--' separator")
			}

			fileSpecs = args[:separatorIndex]
			tags = args[separatorIndex:]
		} else {
			if len(args) < 1 {
				return errors.New("usage: gooru untag <source> [tag1]...")
			}
			fileSpecs = args[0:1]
			tags = args[1:]
		}

		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: untag\n")
			if untagExpressionMode {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed Expression: %v\n", fileSpecs)
			} else {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", fileSpecs)
			}
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed Tags: %v\n", tags)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		var files []string
		var err error
		if untagExpressionMode {
			if len(fileSpecs) == 0 {
				return errors.New("expression mode (-e) requires an expression")
			}
			expression := strings.Join(fileSpecs, " ")
			files, err = svc.ListFilesByQuery(expression, verbose)
			if err != nil {
				return fmt.Errorf("error evaluating expression: %w", err)
			}
		} else {
			files, err = expandFileArgs(fileSpecs)
			if err != nil {
				return fmt.Errorf("error expanding file arguments: %w", err)
			}
		}

		if len(files) == 0 {
			fmt.Println("No files found to untag.")
			return nil
		}

		var filesFailed int
		progressCb := func(filePath string, err error) {
			if err != nil {
				filesFailed++
				fmt.Printf("Failed to process '%s': %v\n", filePath, err)
			} else if untagShowProgress {
				if len(tags) > 0 {
					fmt.Printf("Removed tags from '%s': %s\n", filePath, strings.Join(tags, ", "))
				} else {
					fmt.Printf("Removed all tags from '%s'\n", filePath)
				}
			}
		}

		var opErr error
		if len(tags) == 0 {
			// No tags provided, so clear all tags. This is the same as `settags` with no tags.
			opErr = svc.SetTagsForFiles(files, tags, progressCb)
		} else {
			// Tags provided, so perform a normal untag operation.
			opErr = svc.UntagFiles(files, tags, progressCb)
		}

		if opErr != nil {
			// This will be a DB error that caused a rollback.
			return fmt.Errorf("a database error occurred, all changes have been rolled back: %w", opErr)
		}

		if filesFailed > 0 {
			fmt.Printf("\nWarning: %d file(s) failed to process.\n", filesFailed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(untagCmd)
	untagCmd.Flags().BoolVarP(&multiInputUntag, "multi", "m", false, "Enable multi-input mode (for multiple file paths or expression parts)")
	untagCmd.Flags().BoolVarP(&untagShowProgress, "progress", "p", false, "Show progress for each file processed")
	untagCmd.Flags().BoolVarP(&untagExpressionMode, "expression", "e", false, "Use a query expression instead of file paths")
}
