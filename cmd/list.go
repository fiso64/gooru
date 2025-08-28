/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"errors"
	"fmt"
	"strings"

	"gooru.local/gooru/internal/query"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list [expression]",
	Short: "Lists files based on a tag expression.",
	Long: `Lists files based on a tag expression.
If no expression is provided, all files are listed.
Currently, only simple tags and 'AND' separated tags are supported.
Example: gooru list "tag1 AND tag2"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var paths []string
		var err error

		switch len(args) {
		case 0:
			paths, err = svc.ListAllFiles()
		case 1:
			expression := args[0]
			// This logic will be replaced by the advanced parser from Phase 4
			if strings.Contains(strings.ToUpper(expression), "AND") {
				tags := query.Parse(expression)
				paths, err = svc.ListFilesByTagsAnd(tags)
			} else {
				// Trim spaces in case the user quotes a single tag " tag1 "
				tag := strings.TrimSpace(expression)
				paths, err = svc.ListFilesByTag(tag)
			}
		default:
			return errors.New("list command takes zero or one argument")
		}

		if err != nil {
			return fmt.Errorf("error listing files: %w", err)
		}

		for _, path := range paths {
			fmt.Println(path)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}