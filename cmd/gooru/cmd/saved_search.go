package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/gooru"
	"gooru.local/types"
)

type savedSearchClient interface {
	ListSavedSearchesForUsername(username string) ([]types.SavedSearch, error)
	CreateSavedSearchForUsername(username, name, expression, sort, order string) (types.SavedSearch, error)
	SearchSavedSearchForUsername(username, reference string, verbose bool) ([]types.FileInfo, error)
	ReorderSavedSearchesForUsername(username string, references []string) error
}

func newSavedSearchCmd(client func() savedSearchClient) *cobra.Command {
	var username string
	cmd := &cobra.Command{
		Use:     "saved-search",
		Aliases: []string{"saved-searches"},
		Short:   "Creates and searches saved file queries.",
	}
	cmd.PersistentFlags().StringVar(&username, "user", "", "Username that owns the saved search")
	_ = cmd.MarkPersistentFlagRequired("user")

	var sort string
	var order string
	createCmd := &cobra.Command{
		Use:   "create <name> [expression]",
		Short: "Creates a saved search for a user.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expression := strings.Join(args[1:], " ")
			item, err := client().CreateSavedSearchForUsername(username, args[0], expression, sort, order)
			if err != nil {
				return fmt.Errorf("create saved search: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Query, item.Sort, item.Order)
			return nil
		},
	}
	createCmd.Flags().StringVar(&sort, "sort", "name", "Saved result sort: name, modified, size, or kind")
	createCmd.Flags().StringVar(&order, "order", "asc", "Saved result order: asc or desc")

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Lists a user's saved searches.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			items, err := client().ListSavedSearchesForUsername(username)
			if err != nil {
				return fmt.Errorf("list saved searches: %w", err)
			}
			for _, item := range items {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\t%s\n", item.ID, item.Name, item.Query, item.Sort, item.Order)
			}
			return nil
		},
	}

	reorderCmd := &cobra.Command{
		Use:   "reorder <name-or-id>...",
		Short: "Persists the complete saved-search order for a user.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client().ReorderSavedSearchesForUsername(username, args); err != nil {
				return fmt.Errorf("reorder saved searches: %w", err)
			}
			return nil
		},
	}

	searchCmd := &cobra.Command{
		Use:     "search <name-or-id>",
		Aliases: []string{"run"},
		Short:   "Searches files using a saved search.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := client().SearchSavedSearchForUsername(username, args[0], verbose)
			if err != nil {
				return fmt.Errorf("search saved search: %w", err)
			}
			for _, file := range files {
				fmt.Fprintln(cmd.OutOrStdout(), file.Path)
			}
			return nil
		},
	}

	cmd.AddCommand(createCmd, listCmd, reorderCmd, searchCmd)
	return cmd
}

func init() {
	rootCmd.AddCommand(newSavedSearchCmd(func() savedSearchClient { return svc }))
}

var _ savedSearchClient = (*gooru.Client)(nil)
