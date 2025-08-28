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
	Use:   "tag <filepath> <tag1> [tag2...]",
	Short: "Tags files with one or more tags.",
	Long: `Tags one or more files with the given tags.

Default (single file mode):
  gooru tag /path/to/file.txt tag1 tag2

Multi-file mode (requires -m flag and '--' separator):
  gooru tag -m /path/one.txt /path/two.png -- tag1 tag2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if multiFileTag {
			return handleMultiFileTag(cmd, args)
		}
		return handleSingleFileTag(args)
	},
}

func handleSingleFileTag(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: gooru tag <filepath> <tag1> [tag2...]")
	}
	filePath := args[0]
	tags := args[1:]
	if err := svc.TagFile(filePath, tags); err != nil {
		return fmt.Errorf("error tagging file: %w", err)
	}
	fmt.Printf("Tagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
	return nil
}

func handleMultiFileTag(cmd *cobra.Command, args []string) error {
	separatorIndex := cmd.Flags().ArgsLenAtDash()

	if separatorIndex == -1 {
		return errors.New("usage: gooru tag -m <file1> [file2...] -- <tag1> [tag2...]\n(missing '--' separator)")
	}
	if separatorIndex == 0 {
		return errors.New("no files provided before '--' separator")
	}
	if separatorIndex == len(args) {
		return errors.New("no tags provided after '--' separator")
	}

	files := args[:separatorIndex]
	tags := args[separatorIndex:]

	for _, filePath := range files {
		if err := svc.TagFile(filePath, tags); err != nil {
			fmt.Printf("Failed to tag '%s': %v\n", filePath, err)
			continue // Continue with the next file
		}
		fmt.Printf("Tagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(tagCmd)
	tagCmd.Flags().BoolVarP(&multiFileTag, "multi", "m", false, "Enable multi-file tagging mode")
}