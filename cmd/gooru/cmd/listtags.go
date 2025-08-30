/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	listtagsCount bool
)

// listtagsCmd represents the listtags command
var listtagsCmd = &cobra.Command{
	Use:   "listtags",
	Short: "Lists all unique tags in the database.",
	Long: `Lists all unique tags known to the system.
By default, tags are sorted alphabetically.
With --count, tags are sorted by usage count (descending) and shown in a table.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if listtagsCount {
			tagsWithCounts, err := svc.GetAllTagsWithCounts()
			if err != nil {
				return fmt.Errorf("could not retrieve tags with counts: %w", err)
			}

			if len(tagsWithCounts) == 0 {
				fmt.Println("No tags found in the database.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			defer w.Flush()
			fmt.Fprintln(w, "COUNT\tTAG")
			for _, item := range tagsWithCounts {
				fmt.Fprintf(w, "%d\t%s\n", item.Count, item.Tag)
			}

		} else {
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
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listtagsCmd)
	listtagsCmd.Flags().BoolVarP(&listtagsCount, "count", "c", false, "Show usage counts and sort by count (descending)")
}