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

var (
	multiFileGetTags bool
)

// gettagsCmd represents the gettags command
var gettagsCmd = &cobra.Command{
	Use:   "gettags <file/dir/glob...>",
	Short: "Gets all tags for given files.",
	Long: `Retrieves and lists all tags associated with specific files, directories, or glob patterns.

To provide multiple file arguments, the -m/--multi flag is required.
When using -m, the output is always in the format 'file ;; tags' for consistency.`,
	Args: cobra.MinimumNArgs(1), // Enforce at least one argument
	RunE: func(cmd *cobra.Command, args []string) error {
		if !multiFileGetTags && len(args) > 1 {
			return errors.New("multiple file arguments require the -m/--multi flag")
		}

		files, err := expandFileArgs(args)
		if err != nil {
			return fmt.Errorf("error expanding file arguments: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No matching files found.")
			return nil
		}

		// Use multi-line format if -m is specified OR if more than one file is found from a single arg (e.g. glob).
		if multiFileGetTags || len(files) > 1 {
			for _, filePath := range files {
				tags, err := svc.GetTagsForFile(filePath)
				if err != nil {
					fmt.Printf("%s ;; [ERROR: %v]\n", filePath, err)
					continue
				}
				fmt.Printf("%s ;; %s\n", filePath, strings.Join(tags, ", "))
			}
		} else {
			// Single-file output (original behavior)
			filePath := files[0]
			tags, err := svc.GetTagsForFile(filePath)
			if err != nil {
				return fmt.Errorf("error getting tags for file: %w", err)
			}

			if len(tags) == 0 {
				fmt.Printf("No tags found for '%s'.\n", filePath)
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
	rootCmd.AddCommand(gettagsCmd)
	gettagsCmd.Flags().BoolVarP(&multiFileGetTags, "multi", "m", false, "Use multi-file mode for consistent 'file ;; tags' output")
}