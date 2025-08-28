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

var multiFileSetTags bool

// settagsCmd represents the settags command
var settagsCmd = &cobra.Command{
	Use:   "settags <filepath> [tag1] [tag2...]",
	Short: "Sets the tags for a file, replacing all existing ones.",
	Long: `Sets the tags for a file, replacing any existing tags.
If no tags are provided, all existing tags will be removed.

Default (single file mode):
  gooru settags /path/to/file.txt tag1 tag2

Multi-file mode (requires -m flag and '--' separator):
  gooru settags -m /path/one.txt /path/two.png -- tag1 tag2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if multiFileSetTags {
			return handleMultiFileSetTags(cmd, args)
		}
		return handleSingleFileSetTags(args)
	},
}

func handleSingleFileSetTags(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: gooru settags <filepath> [tag1] [tag2...]")
	}
	filePath := args[0]
	tags := args[1:]
	if err := svc.SetTagsForFile(filePath, tags); err != nil {
		return fmt.Errorf("error setting tags for file: %w", err)
	}
	if len(tags) > 0 {
		fmt.Printf("Set tags for '%s' to: %s\n", filePath, strings.Join(tags, ", "))
	} else {
		fmt.Printf("Removed all tags from '%s'\n", filePath)
	}
	return nil
}

func handleMultiFileSetTags(cmd *cobra.Command, args []string) error {
	separatorIndex := cmd.Flags().ArgsLenAtDash()

	if separatorIndex == -1 {
		return errors.New("usage: gooru settags -m <file1> [file2...] -- [tag1] [tag2...]\n(missing '--' separator)")
	}
	if separatorIndex == 0 {
		return errors.New("no files provided before '--' separator")
	}

	files := args[:separatorIndex]
	tags := args[separatorIndex:]

	for _, filePath := range files {
		if err := svc.SetTagsForFile(filePath, tags); err != nil {
			fmt.Printf("Failed to set tags for '%s': %v\n", filePath, err)
			continue // Continue with the next file
		}
		if len(tags) > 0 {
			fmt.Printf("Set tags for '%s' to: %s\n", filePath, strings.Join(tags, ", "))
		} else {
			fmt.Printf("Removed all tags from '%s'\n", filePath)
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(settagsCmd)
	settagsCmd.Flags().BoolVarP(&multiFileSetTags, "multi", "m", false, "Enable multi-file settags mode")
}