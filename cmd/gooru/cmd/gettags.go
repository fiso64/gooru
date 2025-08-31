/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"errors"
	"fmt"
	"os"

	"gooru.local/gooru/cmd/gooru/display"
	"gooru.local/gooru/types"
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

If a single file is matched, its tags are listed one per line.
If multiple files are matched (via globs, directories, or the -m flag),
the output is a table of paths and their associated tags.`,
	Args: cobra.MinimumNArgs(1), // Enforce at least one argument
	RunE: func(cmd *cobra.Command, args []string) error {
		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: gettags\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Input Args: %v\n", args)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}
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

		// Use table format if -m is specified OR if more than one file is found from a single arg (e.g. glob).
		if multiFileGetTags || len(files) > 1 {
			fileInfos := make([]types.FileInfo, 0, len(files))
			for _, filePath := range files {
				info, status, err := svc.GetFileInfoForFile(filePath)
				if err != nil {
					// Don't pollute table output with errors, send to stderr
					fmt.Fprintf(cmd.ErrOrStderr(), "error processing '%s': %v\n", filePath, err)
					continue
				}
				if status == types.StatusModified {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: '%s' has been modified. Tags for the previous version are not shown in the table.\n", filePath)
				}
				fileInfos = append(fileInfos, info)
			}
			display.PrintTable(fileInfos, "PATH", "TAGS")
		} else {
			// Single-file output with helpful messages.
			filePath := files[0]
			tags, status, err := svc.GetTagsForFile(filePath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("File not found: %s\n", filePath)
					return nil
				}
				return fmt.Errorf("error getting tags for '%s': %w", filePath, err)
			}

			if len(tags) == 0 {
				switch status {
				case types.StatusModified:
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: '%s' has been modified. Tags for the previous version are not shown. Please re-tag the file to update it.\n", filePath)
				case types.StatusNotInDB:
					fmt.Printf("No tags found for '%s'. To track this file (especially if it was moved or renamed), use: 'gooru add \"%s\"'\n", filePath, filePath)
				case types.StatusOK:
					fmt.Printf("No tags found for '%s'.\n", filePath)
				}
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
	gettagsCmd.Flags().BoolVarP(&multiFileGetTags, "multi", "m", false, "Process multiple file arguments and force table output")
}