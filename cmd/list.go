/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"gooru/internal/database"
	"gooru/internal/query"
	"gooru/internal/service"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list [expression]",
	Short: "Lists files based on a tag expression.",
	Long: `Lists files based on a tag expression.
Expressions can be simple tags or AND-separated tags, e.g., "tag1 AND tag2".`,
	Run: func(cmd *cobra.Command, args []string) {
		db, err := database.InitDB("gooru.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		service := service.NewService(db)

		var paths []string

		switch len(args) {
		case 0:
			paths, err = service.ListAllFiles()
		case 1:
			expression := args[0]
			if strings.Contains(expression, "AND") {
				tags := query.Parse(expression)
				paths, err = service.ListFilesByTagsAnd(tags)
			} else {
				paths, err = service.ListFilesByTag(expression)
			}
		default:
			fmt.Println("Usage: gooru list [expression]")
			os.Exit(1)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing files: %v\n", err)
			os.Exit(1)
		}

		for _, path := range paths {
			fmt.Println(path)
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
