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
	Use:   "untag <filepath> <tag1> [tag2...]",
	Short: "Removes one or more tags from files.",
	Long: `Removes one or more tags from the given files.

Default (single file mode):
  gooru untag /path/to/file.txt tag1 tag2

Multi-file mode (requires -m flag and '--' separator):
  gooru untag -m /path/one.txt /path/two.png -- tag1 tag2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if multiFileUntag {
			return handleMultiFileUntag(cmd, args)
		}
		return handleSingleFileUntag(args)
	},
}

func handleSingleFileUntag(args []string) error {
	if len(args) < 2 {
		return errors.New("usage: gooru untag <filepath> <tag1> [tag2...]")
	}
	filePath := args[0]
	tags := args[1:]
	if err := svc.UntagFile(filePath, tags); err != nil {
		return fmt.Errorf("error untagging file: %w", err)
	}
	fmt.Printf("Untagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
	return nil
}

func handleMultiFileUntag(cmd *cobra.Command, args []string) error {
	separatorIndex := cmd.Flags().ArgsLenAtDash()

	if separatorIndex == -1 {
		return errors.New("usage: gooru untag -m <file1> [file2...] -- <tag1> [tag2...]\n(missing '--' separator)")
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
		if err := svc.UntagFile(filePath, tags); err != nil {
			fmt.Printf("Failed to untag '%s': %v\n", filePath, err)
			continue // Continue with the next file
		}
		fmt.Printf("Untagged '%s' with: %s\n", filePath, strings.Join(tags, ", "))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(untagCmd)
	untagCmd.Flags().BoolVarP(&multiFileUntag, "multi", "m", false, "Enable multi-file untagging mode")
}