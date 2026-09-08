/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// editpathCmd represents the editpath command
var editpathCmd = &cobra.Command{
	Use:   "editpath <oldpath> <newpath>",
	Short: "Manually updates a file path in the database.",
	Long: `Manually updates a file path in the database.

This is useful for tracking a file that has been renamed or moved
without needing to perform a full 'relink' scan. The content hash
is not affected.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: editpath\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Input Args: %v\n", args)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}
		oldPath := args[0]
		newPath := args[1]

		err := svc.EditPath(oldPath, newPath)
		if err != nil {
			return fmt.Errorf("could not update path: %w", err)
		}

		fmt.Printf("Updated path from '%s' to '%s'\n", oldPath, newPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editpathCmd)
}
