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

var multiFileTag bool

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

		files, err := expandFileArgs(fileSpecs)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No files found to tag.")
			return nil
		}

		for _, filePath := range files {
			if err := svc.TagFile(filePath, tags); err != nil {
				fmt.Printf("Failed to tag '%s': %v\n", filePath, err)
				continue // Continue with the next file
			}
			fmt.Printf("Tagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.Flags().BoolVarP(&multiFileTag, "multi", "m", false, "Enable multi-file tagging mode")
}