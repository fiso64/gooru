/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// relinkallCmd represents the relinkall command
var relinkallCmd = &cobra.Command{
	Use:   "relinkall <dir1> [dir2...]",
	Short: "Scans directories and updates locations for known files.",
	Long: `Performs a fast check to see if any known files within the given directories
have moved or changed. If they have, it performs a full, high-performance
scan to update the database.

This command does NOT add new files to the database; it only "re-links" existing content.`,
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
		stats, err := svc.Relink(args)
		if err != nil {
			return fmt.Errorf("error during relink process: %w", err)
		}

		fmt.Printf("Scan complete.\n")
		fmt.Printf("  - Files Scanned: %d\n", stats.FilesScanned)
		fmt.Printf("  - Locations Updated/Added: %d\n", stats.LocationsAdded)
		fmt.Printf("  - Obsolete Locations Removed: %d\n", stats.LocationsRemoved)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(relinkallCmd)
}