/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/display"
	"gooru.local/types"
)

// tableCmd represents the table command
var tableCmd = &cobra.Command{
	Use:   "table [expression]",
	Short: "Lists files and their tags in a table.",
	Long: `Lists files and their tags in a table based on a tag expression.
If no expression is provided, all files are listed.

Expressions support AND, OR, and NOT logic with grouping.
- Simple tag:     gooru table video
- Implicit AND:   gooru table "video family"
- Explicit AND:   gooru table "video & family"
- OR:             gooru table "video | photo"
- NOT (EXCEPT):   gooru table "family - work"
- Grouping:       gooru table "(photo | video) holiday -work"
- By extension:   gooru table "ext:mp4 | ext:mov"
- By type:        gooru table "type:img holiday"
- Meta-queries:   gooru table "@tagged"          (all files with any tag)
                  gooru table "-@tagged"         (all files with no tags)
                  gooru table "location"         (files with a 'location' tag, any value)
                  gooru table "location:"        (files with simple 'location' tag)
                  gooru table "location:*"      (files with any non-empty 'location' tag)
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var files []types.FileInfo
		var err error

		expression := ""
		if len(args) > 0 {
			expression = strings.Join(args, " ")
		}

		if expression == "" {
			files, err = svc.GetAllFilesInfo()
		} else {
			files, err = svc.GetFilesInfoByQuery(expression, verbose)
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
