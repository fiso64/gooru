/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"

	"gooru/internal/database"
	"gooru/internal/service"
	"github.com/spf13/cobra"
)

// gettagsCmd represents the gettags command
var gettagsCmd = &cobra.Command{
	Use:   "gettags <filepath>",
	Short: "Gets all tags for a given file.",
	Long:  `Gets all tags for a given file.`, // Note: Using backticks for multiline string literal here, which is idiomatic Go and handles newlines correctly without explicit escaping.
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) != 1 {
			fmt.Println("Usage: gooru gettags <filepath>")
			os.Exit(1)
		}

		db, err := database.InitDB("gooru.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close()

		service := service.NewService(db)

		filePath := args[0]

		tags, err := service.GetTagsForFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting tags for file: %v\n", err)
			os.Exit(1)
		}

		for _, tag := range tags {
			fmt.Println(tag)
		}
	},
}

func init() {
	rootCmd.AddCommand(gettagsCmd)
}
