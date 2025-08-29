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
	multiFileSetTags    bool
	setTagsShowProgress bool
)

// settagsCmd represents the settags command
var settagsCmd = &cobra.Command{
	Use:   "settags <file/dir/glob> [tag1] [tag2...]",
	Short: "Sets the tags for files, replacing all existing ones.",
	Long: `Sets the tags for files, replacing any existing tags.
File arguments can be paths, directories, or glob patterns.
If no tags are provided, all existing tags will be removed.

Default mode:
  gooru settags "archive/*.zip" archived

Multi-file mode (for complex file lists):
  gooru settags -m /path/one.txt /path/two.png -- tag1 tag2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var fileSpecs []string
		var tags []string

		if multiFileSetTags {
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
				return errors.New("usage: gooru settags <file/dir/glob> [tag1] [tag2...]")
			}
			fileSpecs = args[0:1]
			tags = args[1:]
		}

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

		return nil
	},
}

func init() {
	rootCmd.AddCommand(settagsCmd)
	settagsCmd.Flags().BoolVarP(&multiFileSetTags, "multi", "m", false, "Enable multi-file settags mode")
	settagsCmd.Flags().BoolVarP(&setTagsShowProgress, "progress", "p", false, "Show progress for each file processed")
}