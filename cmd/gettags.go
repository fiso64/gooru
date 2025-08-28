/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// gettagsCmd represents the gettags command
var gettagsCmd = &cobra.Command{
	Use:   "gettags <file/dir/glob...>",
	Short: "Gets all tags for given files.",
	Long:  `Retrieves and lists all tags associated with specific files, directories, or glob patterns.`,
	Args:  cobra.MinimumNArgs(1), // Enforce at least one argument
	RunE: func(cmd *cobra.Command, args []string) error {
		files, err := expandFileArgs(args)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No matching files found.")
			return nil
		}

		if len(files) > 1 {
			// Multi-file output
			for _, filePath := range files {
				tags, err := svc.GetTagsForFile(filePath)
				if err != nil {
					fmt.Printf("%s: [ERROR: %v]\n", filePath, err)
					continue
				}
				fmt.Printf("%s: %s\n", filePath, strings.Join(tags, ", "))
			}
		} else {
			// Single-file output (original behavior)
			filePath := files[0]
			tags, err := svc.GetTagsForFile(filePath)
			if err != nil {
				return fmt.Errorf("error getting tags for file: %w", err)
			}

			if len(tags) == 0 {
				fmt.Printf("No tags found for '%s'.\n", filePath)
				return nil
			}

			for _, tag := range tags {
				fmt.Println(tag)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gettagsCmd)
}