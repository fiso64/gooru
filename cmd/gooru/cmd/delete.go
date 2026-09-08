/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/display"
)

var (
	deleteExpressionMode bool
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete <file/dir/glob...>",
	Short: "Deletes file records from the database.",
	Long: `Deletes file records from the database.

The source can be file paths, directories, or glob patterns.
For query-based deletion, use the -e flag.

This command removes the file's location record. If this is the last known
location for a piece of content, the content and its associated tags are
also permanently removed from the database. This action cannot be undone.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: delete\n")
			if deleteExpressionMode {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed Expression: %v\n", args)
			} else {
				fmt.Fprintf(cmd.OutOrStderr(), "Parsed File Specs: %v\n", args)
			}
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}

		if deleteExpressionMode {
			expression := strings.Join(args, " ")
			affected, err := svc.DeleteFilesByQuery(expression)
			if err != nil {
				return fmt.Errorf("a database error occurred: %w", err)
			}
			fmt.Printf("Deleted %d file record(s) matching expression.\n", affected)
		} else {
			expansionResult, err := expandFileArgs(args)
			if err != nil {
				return fmt.Errorf("error expanding file arguments: %w", err)
			}
			if len(expansionResult.NotFound) > 0 {
				for _, notFound := range expansionResult.NotFound {
					display.Errorf("path not found: %s", notFound)
				}
				// Fail fast for destructive commands
				return fmt.Errorf("aborted due to path errors")
			}

			files := expansionResult.Found
			if len(files) == 0 {
				fmt.Println("No matching files found to delete.")
				return nil
			}

			// PruneLocations is the service method for deleting by path.
			affected, err := svc.PruneLocations(files)
			if err != nil {
				return fmt.Errorf("a database error occurred: %w", err)
			}

			fmt.Printf("Deleted %d file record(s).\n", affected)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVarP(&deleteExpressionMode, "expression", "e", false, "Use a query expression instead of file paths")
}
