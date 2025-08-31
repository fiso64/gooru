/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"gooru.local/gooru/cmd/gooru/display"
	"gooru.local/gooru/types"
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
It will propose a set of changes (moves, new duplicate locations, deletions)
and ask for a single confirmation before applying them.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if verbose {
			fmt.Fprintf(cmd.OutOrStderr(), "--- VERBOSE ---\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Command: relinkall\n")
			fmt.Fprintf(cmd.OutOrStderr(), "Input Dirs: %v\n", args)
			fmt.Fprintf(cmd.OutOrStderr(), "---------------\n")
		}
		fmt.Println("Checking for changed or moved files...")
		needsScan, err := svc.NeedsRelink(args)
		if err != nil {
			return fmt.Errorf("error during pre-check: %w", err)
		}

		if !needsScan {
			fmt.Println("All file locations are up-to-date.")
			return nil
		}

		fmt.Println("Changes detected. Starting full scan to find proposed changes...")
		result, err := svc.Relink(args)
		if err != nil {
			return fmt.Errorf("error during relink scan: %w", err)
		}

		hasMoves := len(result.ProposedMoves) > 0
		hasAdds := len(result.ProposedAdds) > 0
		hasDeletes := len(result.ProposedDeletes) > 0

		if !hasMoves && !hasAdds && !hasDeletes {
			fmt.Println("Scan complete. All file locations are consistent.")
			return nil
		}

		fmt.Println("\nScan complete. The following changes are proposed:")

		if hasMoves {
			fmt.Println("\n[MOVED / RENAMED]")
			for _, move := range result.ProposedMoves {
				fmt.Printf("  %s  ->  %s\n", move.OldPath, move.NewLocation.Path)
			}
		}

		if hasAdds {
			fmt.Println("\n[NEW LOCATIONS / DUPLICATES]")
			// Convert LocationInfo to FileInfo for display
			addInfos := make([]types.FileInfo, len(result.ProposedAdds))
			for i, add := range result.ProposedAdds {
				addInfos[i] = types.FileInfo{Path: add.Path, Size: add.Size, Tags: add.TagsCache}
			}
			display.PrintTable(addInfos)
		}

		if hasDeletes {
			fmt.Println("\n[DELETED FROM DISK]")
			display.PrintTable(result.ProposedDeletes)
		}

		doApply := false
		if relinkYes {
			doApply = true
		} else if relinkNo {
			doApply = false
			fmt.Println("\nChanges not applied due to --no flag.")
		} else {
			doApply, err = confirmApply(
				len(result.ProposedMoves),
				len(result.ProposedAdds),
				len(result.ProposedDeletes),
			)
			if err != nil {
				return fmt.Errorf("could not get user confirmation: %w", err)
			}
		}

		if !doApply {
			fmt.Println("No changes were applied.")
			return nil
		}

		stats, err := svc.ApplyRelinkChanges(result)
		if err != nil {
			return fmt.Errorf("failed to apply changes: %w", err)
		}

		fmt.Printf("\nChanges applied successfully.\n")
		fmt.Printf("  - Files Scanned: %d\n", result.Stats.FilesScanned)
		fmt.Printf("  - Locations Updated/Added: %d\n", stats.LocationsAdded)
		fmt.Printf("  - Obsolete Locations Removed: %d\n", stats.LocationsRemoved)

		return nil
	},
}

func confirmApply(moves, adds, deletes int) (bool, error) {
	var summaryParts []string
	if moves > 0 {
		s := "paths"
		if moves == 1 {
			s = "path"
		}
		summaryParts = append(summaryParts, fmt.Sprintf("update %d %s", moves, s))
	}
	if adds > 0 {
		s := "locations"
		if adds == 1 {
			s = "location"
		}
		summaryParts = append(summaryParts, fmt.Sprintf("add %d %s", adds, s))
	}
	if deletes > 0 {
		s := "records"
		if deletes == 1 {
			s = "record"
		}
		summaryParts = append(summaryParts, fmt.Sprintf("remove %d %s", deletes, s))
	}

	summary := ""
	if len(summaryParts) > 0 {
		summary = fmt.Sprintf(" (This will %s)", strings.Join(summaryParts, ", "))
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nApply these changes?%s [Y/n]: ", summary)
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