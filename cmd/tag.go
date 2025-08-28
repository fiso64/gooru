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

// tagCmd represents the tag command
var tagCmd = &cobra.Command{
	Use:   "tag <filepath> <tag1> [tag2...]",
	Short: "Tags a file with one or more tags.",
	Long: `Tags a file with one or more tags. 
The first argument is the file path, and all subsequent arguments are the tags.`,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 2 {
			fmt.Println("Usage: gooru tag <filepath> <tag1> [tag2...]")
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
		tags := args[1:]

		if err := service.TagFile(filePath, tags); err != nil {
			fmt.Fprintf(os.Stderr, "Error tagging file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully tagged '%s' with: %v\n", filePath, tags)
	},
}

func init() {
	rootCmd.AddCommand(tagCmd)
}
