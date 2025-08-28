/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// listtagsCmd represents the listtags command
var listtagsCmd = &cobra.Command{
	Use:   "listtags",
	Short: "Lists all unique tags in the database.",
	Long:  `Lists all unique tags known to the system, sorted alphabetically.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tags, err := svc.GetAllTags()
		if err != nil {
			return fmt.Errorf("could not retrieve tags: %w", err)
		}

		if len(tags) == 0 {
			fmt.Println("No tags found in the database.")
			return nil
		}

		for _, tag := range tags {
			fmt.Println(tag)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listtagsCmd)
}