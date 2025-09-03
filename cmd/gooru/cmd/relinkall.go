/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/display"
	"gooru.local/types"
)

var (
	relinkYes          bool
	relinkMoves        bool
	relinkAdds         bool
	relinkDeletes      bool
	relinkVerifyHashes bool
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
		needsScan, err := svc.NeedsRelink(args, relinkVerifyHashes)
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

		changesToApply := types.RelinkResult{
			Stats: result.Stats, // Preserve stats for the final report.
		}

		// If no specific action flags are provided, all actions are considered.
		applyAll := !relinkMoves && !relinkAdds && !relinkDeletes
		applyMoves := relinkMoves || applyAll
		applyAdds := relinkAdds || applyAll
		applyDeletes := relinkDeletes || applyAll

		if relinkYes {
			// Non-interactive: Print all changes first, then apply selected ones.
			if hasMoves {
				fmt.Println("\n[MOVED / RENAMED]")
				for _, move := range result.ProposedMoves {
					fmt.Printf("  %s  ->  %s\n", move.OldPath, move.NewLocation.Path)
				}
			}
			if hasAdds {
				fmt.Println("\n[NEW LOCATIONS / DUPLICATES]")
				addInfos := make([]types.FileInfo, len(result.ProposedAdds))
				for i, add := range result.ProposedAdds {
					var tags []string
					if add.TagsCache != "" {
						tags = strings.Split(add.TagsCache, " ")
					}
					addInfos[i] = types.FileInfo{Path: add.Path, Size: add.Size, Tags: tags}
				}
				display.PrintTable(addInfos)
			}
			if hasDeletes {
				fmt.Println("\n[DELETED FROM DISK]")
				display.PrintTable(result.ProposedDeletes)
			}

			if hasMoves && applyMoves {
				changesToApply.ProposedMoves = result.ProposedMoves
			}
			if hasAdds && applyAdds {
				changesToApply.ProposedAdds = result.ProposedAdds
			}
			if hasDeletes && applyDeletes {
				changesToApply.ProposedDeletes = result.ProposedDeletes
			}
		} else {
			// Interactive: Interleave printing and confirmation.
			if hasMoves && applyMoves {
				fmt.Println("\n[MOVED / RENAMED]")
				for _, move := range result.ProposedMoves {
					fmt.Printf("  %s  ->  %s\n", move.OldPath, move.NewLocation.Path)
				}
				confirm, err := confirmChange("moves", len(result.ProposedMoves))
				if err != nil {
					return err
				}
				if confirm {
					changesToApply.ProposedMoves = result.ProposedMoves
				}
			}

			if hasAdds && applyAdds {
				fmt.Println("\n[NEW LOCATIONS / DUPLICATES]")
				addInfos := make([]types.FileInfo, len(result.ProposedAdds))
				for i, add := range result.ProposedAdds {
					var tags []string
					if add.TagsCache != "" {
						tags = strings.Split(add.TagsCache, " ")
					}
					addInfos[i] = types.FileInfo{Path: add.Path, Size: add.Size, Tags: tags}
				}
				display.PrintTable(addInfos)

				confirm, err := confirmChange("adds", len(result.ProposedAdds))
				if err != nil {
					return err
				}
				if confirm {
					changesToApply.ProposedAdds = result.ProposedAdds
				}
			}

			if hasDeletes && applyDeletes {
				fmt.Println("\n[DELETED FROM DISK]")
				display.PrintTable(result.ProposedDeletes)

				confirm, err := confirmChange("deletes", len(result.ProposedDeletes))
				if err != nil {
					return err
				}
				if confirm {
					changesToApply.ProposedDeletes = result.ProposedDeletes
				}
			}
		}

		if len(changesToApply.ProposedMoves) == 0 && len(changesToApply.ProposedAdds) == 0 && len(changesToApply.ProposedDeletes) == 0 {
			fmt.Println("\nNo changes were applied.")
			return nil
		}

		stats, err := svc.ApplyRelinkChanges(changesToApply)
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

func confirmChange(changeType string, count int) (bool, error) {
	var action, item string
	switch changeType {
	case "moves":
		action = "Update"
		item = "path"
		if count > 1 {
			item = "paths"
		}
	case "adds":
		action = "Add"
		item = "location"
		if count > 1 {
			item = "locations"
		}
	case "deletes":
		action = "Remove"
		item = "record"
		if count > 1 {
			item = "records"
		}
	default:
		return false, fmt.Errorf("unknown change type: %s", changeType)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nApply %d proposed %s? (This will %s %d %s) [Y/n]: ", count, changeType, strings.ToLower(action), count, item)
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
	relinkallCmd.Flags().BoolVarP(&relinkYes, "yes", "y", false, "Automatically confirm and apply all proposed changes.")
	relinkallCmd.Flags().BoolVar(&relinkMoves, "moves", false, "Only apply proposed moves/renames.")
	relinkallCmd.Flags().BoolVar(&relinkAdds, "adds", false, "Only apply proposed new locations/duplicates.")
	relinkallCmd.Flags().BoolVar(&relinkDeletes, "deletes", false, "Only apply proposed deletions of obsolete records.")
	relinkallCmd.Flags().BoolVar(&relinkVerifyHashes, "always-verify-hash", false, "Force content hashing for all files during pre-scan for 100% accuracy (slower)")
}