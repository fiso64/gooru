/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gooru.local/gooru/internal/display"
	"github.com/spf13/cobra"
)

var (
	relinkYes bool
	relinkNo  bool
)

// relinkallCmd represents the relinkall command
var relinkallCmd = &cobra.Command{
	Use:   "relinkall <dir1> [dir2...]",
	Short: "Scans directories and updates locations for known files.",
	Long: `Performs a fast check to see if any known files within the given directories
have moved or changed. If they have, it performs a full, high-performance
scan to update the database.

This command does NOT add new files to the database; it only "re-links" existing content.
If it finds database entries for files that no longer exist, it will prompt for deletion.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Checking for changed or moved files...")
		needsScan, err := svc.NeedsRelink(args)
		if err != nil {
			return fmt.Errorf("error during pre-check: %w", err)
		}

		if !needsScan {
			fmt.Println("All file locations are up-to-date.")
			return nil
		}

		fmt.Println("Changes detected. Starting full scan to update locations...")
		result, err := svc.Relink(args)
		if err != nil {
			return fmt.Errorf("error during relink process: %w", err)
		}

		stats := result.Stats

		if len(result.UnrelinkedFiles) > 0 {
			fmt.Printf("\nFound %d database entries for files that could not be found on disk:\n", len(result.UnrelinkedFiles))
			display.PrintTable(result.UnrelinkedFiles)

			doDelete := false
			if relinkYes {
				doDelete = true
			} else if relinkNo {
				doDelete = false
			} else {
				doDelete, err = confirmDeletion()
				if err != nil {
					return fmt.Errorf("could not get user confirmation: %w", err)
				}
			}

			if doDelete {
				pathsToDelete := make([]string, len(result.UnrelinkedFiles))
				for i, file := range result.UnrelinkedFiles {
					pathsToDelete[i] = file.Path
				}
				removedCount, err := svc.PruneLocations(pathsToDelete)
				if err != nil {
					return fmt.Errorf("failed to remove obsolete locations: %w", err)
				}
				stats.LocationsRemoved = removedCount
			}
		}

		fmt.Printf("\nScan complete.\n")
		fmt.Printf("  - Files Scanned: %d\n", stats.FilesScanned)
		fmt.Printf("  - Locations Updated/Added: %d\n", stats.LocationsAdded)
		fmt.Printf("  - Obsolete Locations Removed: %d\n", stats.LocationsRemoved)

		return nil
	},
}

func confirmDeletion() (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nDelete these entries from the database? [Y/n]: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	input = strings.TrimSpace(strings.ToLower(input))
	// Default to "yes" if user just presses Enter
	return input == "y" || input == "", nil
}

func init() {
	rootCmd.AddCommand(relinkallCmd)
	relinkallCmd.Flags().BoolVarP(&relinkYes, "yes", "y", false, "Automatically confirm deletion of unlinked file entries.")
	relinkallCmd.Flags().BoolVarP(&relinkNo, "no", "n", false, "Automatically deny deletion of unlinked file entries.")
	relinkallCmd.MarkFlagsMutuallyExclusive("yes", "no")
}