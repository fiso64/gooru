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

// ErrExitCode1 is a sentinel error used to signal that the command should exit with code 1,
// without printing an error message. This is used for commands like 'exists' to indicate 'false'.
var ErrExitCode1 = errors.New("exit code 1")

// existsCmd represents the exists command
var existsCmd = &cobra.Command{
	Use:   "exists [expression]",
	Short: "Checks if any files match a tag expression.",
	Long: `Checks if any files match a tag expression and prints "true" or "false".
It exits with code 0 if files are found ("true") and 1 if not ("false"),
making it ideal for shell scripting.

This is a high-performance check that stops searching as soon as a single match is found.

Example:
  if gooru exists "photo vacation"; then
    echo "Vacation photos found!"
  fi

Meta-queries are also supported:
  if gooru exists "@tagged"; then ...
  if gooru exists "-@tagged"; then ...
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		expression := ""
		if len(args) > 0 {
			expression = strings.Join(args, " ")
		}

		found, err := svc.ExistsFilesByQuery(expression, verbose)
		if err != nil {
			return fmt.Errorf("error checking for files: %w", err)
		}

		if found {
			fmt.Println("true")
			return nil
		}

		fmt.Println("false")
		return ErrExitCode1
	},
}

func init() {
	rootCmd.AddCommand(existsCmd)
}