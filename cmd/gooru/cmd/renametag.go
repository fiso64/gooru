/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// renametagCmd represents the renametag command
var renametagCmd = &cobra.Command{
	Use:   "renametag <old_name> <new_name>",
	Short: "Renames a tag across the entire database.",
	Long: `Renames an existing tag to a new name everywhere it is used.

This operation is atomic. It will fail if the new tag name already exists
or if the old tag name is not found.

Example:
  gooru renametag "project:alpha" "project:beta"
  gooru renametag draft final`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldName := args[0]
		newName := args[1]

		err := svc.RenameTag(oldName, newName)
		if err != nil {
			return err // The service layer error is user-friendly enough
		}

		fmt.Printf("Renamed tag '%s' to '%s'.\n", oldName, newName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(renametagCmd)
}
