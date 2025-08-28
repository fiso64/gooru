/*
Copyright © 2025 Your Name
*/
package cmd

import (
    "errors"
    "fmt"
    "strings"

    "gooru.local/gooru/internal/display"
    "gooru.local/gooru/internal/query"
    "gooru.local/gooru/internal/types"
    "github.com/spf13/cobra"
)

// tableCmd represents the table command
var tableCmd = &cobra.Command{
    Use:   "table [expression]",
    Short: "Lists files and their tags in a table.",
    Long: `Lists files and their associated data in a formatted table based on a tag expression.
If no expression is provided, all files are listed.
Example: gooru table "tag1 AND tag2"`,
    RunE: func(cmd *cobra.Command, args []string) error {
        var files []types.FileInfo
        var err error

        switch len(args) {
        case 0:
            files, err = svc.GetAllFilesInfo()
        case 1:
            expression := args[0]
            // This logic will be replaced by the advanced parser from Phase 4
            if strings.Contains(strings.ToUpper(expression), "AND") {
                tags := query.Parse(expression)
                files, err = svc.GetFilesInfoByTagsAnd(tags)
            } else {
                // Trim spaces in case the user quotes a single tag " tag1 "
                tag := strings.TrimSpace(expression)
                files, err = svc.GetFilesInfoByTag(tag)
            }
        default:
            return errors.New("table command takes zero or one argument")
        }

        if err != nil {
            return fmt.Errorf("error listing files: %w", err)
        }

        display.PrintTable(files)
        return nil
    },
}

func init() {
    rootCmd.AddCommand(tableCmd)
}