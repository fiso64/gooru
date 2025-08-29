/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
    Use:   "list [expression]",
    Short: "Lists files based on a tag expression.",
    Long: `Lists files based on a tag expression.
If no expression is provided, all files are listed.

Expressions support AND, OR, and NOT logic with grouping.
- Simple tag:     gooru list video
- Implicit AND:   gooru list "video family"
- Explicit AND:   gooru list "video & family"
- OR:             gooru list "video | photo"
- NOT (EXCEPT):   gooru list "family - work"
- Grouping:       gooru list "(photo | video) holiday -work"
- By extension:   gooru list "ext:mp4 | ext:mov"
- By type:        gooru list "type:img holiday"`,
    RunE: func(cmd *cobra.Command, args []string) error {
        var paths []string
        var err error

        expression := ""
        if len(args) > 0 {
            // Join all args to form the expression, supporting `gooru list cat outside`
            expression = strings.Join(args, " ")
        }

        if expression == "" {
            paths, err = svc.ListAllFiles()
        } else {
            paths, err = svc.ListFilesByQuery(expression, debug)
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