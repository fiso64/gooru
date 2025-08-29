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

var multiFileUntag bool

// untagCmd represents the untag command
var untagCmd = &cobra.Command{
	Use:   "untag <file/dir/glob> <tag1> [tag2...]",
	Short: "Removes one or more tags from files.",
	Long: `Removes one or more tags from files. File arguments can be paths, directories, or glob patterns.

Default mode:
  gooru untag <file/dir/glob> <tag1> [tag2...]
  Example: gooru untag "tmp/*" temporary

Multi-file mode (for complex file lists):
  gooru untag -m /path/one.txt /path/two.png -- tag1 tag2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var fileSpecs []string
		var tags []string

		if multiFileUntag {
			separatorIndex := cmd.Flags().ArgsLenAtDash()

			if separatorIndex == -1 {
				return errors.New("usage: gooru untag -m <file1>... -- <tag1>...\n(missing '--' separator)")
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
				return errors.New("usage: gooru untag <file/dir/glob> <tag1> [tag2...]")
			}
			fileSpecs = args[0:1]
			tags = args[1:]
		}

		files, err := expandFileArgs(fileSpecs)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No files found to untag.")
			return nil
		}

		var filesFailed int
		progressCb := func(filePath string, err error) {
			if err != nil {
				filesFailed++
				fmt.Printf("Failed to untag '%s': %v\n", filePath, err)
			} else {
				fmt.Printf("Untagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
			}
		}

		if err := svc.UntagFiles(files, tags, progressCb); err != nil {
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
	rootCmd.AddCommand(untagCmd)
	untagCmd.Flags().BoolVarP(&multiFileUntag, "multi", "m", false, "Enable multi-file untagging mode")
}