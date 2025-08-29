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
	multiFileTag    bool
	tagShowProgress bool
)

// tagCmd represents the tag command
var tagCmd = &cobra.Command{
	Use:   "tag <file/dir/glob> <tag1> [tag2...]",
	Short: "Tags files with one or more tags.",
	Long: `Tags files with the given tags. File arguments can be paths, directories, or glob patterns.

Default mode:
  gooru tag <file/dir/glob> <tag1> [tag2...]
  Example: gooru tag "photos/*.jpg" vacation summer

Multi-file mode (for complex file lists):
  gooru tag -m /path/one.txt /path/two.png -- tag1 tag2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var fileSpecs []string
		var tags []string

		if multiFileTag {
			separatorIndex := cmd.Flags().ArgsLenAtDash()

			if separatorIndex == -1 {
				return errors.New("usage: gooru tag -m <file1>... -- <tag1>...\n(missing '--' separator)")
			}
			if separatorIndex == 0 {
				return errors.New("no files provided before '--' separator")
			}
			if separatorIndex == len(args) {
				return errors.New("no tags provided after '--' separator")
			}

			fileSpecs = args[:separatorIndex]
			tags = args[separatorIndex:]
		} else {
			if len(args) < 2 {
				return errors.New("usage: gooru tag <file/dir/glob> <tag1> [tag2...]")
			}
			fileSpecs = args[0:1]
			tags = args[1:]
		}

		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: tag\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", fileSpecs)
			fmt.Fprintf(cmd.OutOrStderr(), "Parsed Tags: %v\n", tags)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		files, err := expandFileArgs(fileSpecs)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No files found to tag.")
			return nil
		}

		var filesFailed int
		progressCb := func(filePath string, err error) {
			if err != nil {
				filesFailed++
				fmt.Printf("Failed to tag '%s': %v\n", filePath, err)
			} else if tagShowProgress {
				fmt.Printf("Tagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
			}
		}

		if err := svc.TagFiles(files, tags, progressCb); err != nil {
			// This will be a DB error that caused a rollback.
			return fmt.Errorf("a database error occurred, all changes have been rolled back: %w", err)
		}

		if filesFailed > 0 {
			fmt.Printf("\nWarning: %d file(s) failed to process.\n", filesFailed)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.Flags().BoolVarP(&multiFileTag, "multi", "m", false, "Enable multi-file tagging mode")
	tagCmd.Flags().BoolVarP(&tagShowProgress, "progress", "p", false, "Show progress for each file processed")
}