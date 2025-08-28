/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// gettagsCmd represents the gettags command
var gettagsCmd = &cobra.Command{
	Use:   "gettags <filepath>",
	Short: "Gets all tags for a given file.",
	Long:  `Retrieves and lists all tags associated with a specific file.`,
	Args:  cobra.ExactArgs(1), // Enforce exactly one argument
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]
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
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gettagsCmd)
}