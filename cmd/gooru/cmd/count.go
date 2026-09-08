/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// countCmd represents the count command
var countCmd = &cobra.Command{
	Use:   "count [expression]",
	Short: "Counts files based on a tag expression.",
	Long: `Counts files based on a tag expression.
If no expression is provided, the total number of files is counted.

For simple, single-tag queries (e.g., 'gooru count photo'), this command
is extremely fast as it uses pre-calculated statistics. For complex
expressions, it runs a full query to get an exact count.

Expressions support AND, OR, and NOT logic with grouping.
- Simple tag:     gooru count video
- Implicit AND:   gooru count "video family"
- OR:             gooru count "video | photo"
- NOT (EXCEPT):   gooru count "family - work"
- Meta-queries:   gooru count "@tagged"
                  gooru count "location:*"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var count int
		var err error

		expression := ""
		if len(args) > 0 {
			expression = strings.Join(args, " ")
		}

		count, err = svc.CountFilesByQuery(expression, verbose)
		if err != nil {
			return fmt.Errorf("error counting files: %w", err)
		}

		fmt.Println(count)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(countCmd)
}
