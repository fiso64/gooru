/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/gooru"
	"gooru.local/types"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes a new Gooru database.",
	Long: `Initializes a new Gooru database at the configured location.

The default remains ~/.config/gooru/gooru.db. Use the persistent --database flag
when initializing an independent instance database.

This is the first command you must run. It will prompt you to choose a hashing
strategy for the new database. This choice is permanent and cannot be changed later.

- Partial Hashing: Very fast for large files (e.g., videos, disk images). It identifies files
  by their size plus hashes of a few small chunks of data. It's reliable enough for
  most use cases, but there's a theoretical possibility of a hash collision.

- Full Hashing: Slower for large files but provides maximum reliability. It reads and
  hashes the entire file content. Choose this if you need guaranteed content integrity
  for critical documents or archives.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := configuredDatabasePath()
		if err != nil {
			return fmt.Errorf("failed to get db path: %w", err)
		}

		// Check if a file already exists at the path. We should not proceed if it does.
		if _, err := os.Stat(dbPath); err == nil {
			return fmt.Errorf("database file already exists at: %s\nTo start over, delete this file and run 'init' again", dbPath)
		} else if !os.IsNotExist(err) {
			// If os.Stat returned an error other than NotExist, it's a problem.
			return fmt.Errorf("failed to check for existing database: %w", err)
		}

		fmt.Println("Choose a hashing strategy for the new database.")
		fmt.Println("This choice is permanent and affects performance vs. reliability trade-offs.")
		fmt.Println()
		fmt.Println("1) Partial Hashing (Recommended, Faster for large files)")
		fmt.Println("   - Hashes file size + strategic chunks. Ideal for media libraries.")
		fmt.Println()
		fmt.Println("2) Full Hashing (Most Reliable, Slower for large files)")
		fmt.Println("   - Hashes the entire file content. Best for critical documents.")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		var strategy types.HashingStrategy
		for {
			fmt.Print("Enter your choice (1 or 2): ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "1" {
				strategy = types.StrategyPartial
				break
			} else if input == "2" {
				strategy = types.StrategyFull
				break
			} else {
				fmt.Println("Invalid choice. Please enter 1 or 2.")
			}
		}

		if err := ensureDatabaseParent(dbPath); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}

		fmt.Printf("\nInitializing new database at %s with '%s' hashing...\n", dbPath, strategy)
		err = gooru.Init(dbPath, strategy, verbose)
		if err != nil {
			return fmt.Errorf("database initialization failed: %w", err)
		}

		fmt.Println("Database initialized successfully.")
		return nil
	},
}

func ensureDatabaseParent(dbPath string) error {
	return os.MkdirAll(filepath.Dir(dbPath), 0700)
}

func init() {
	rootCmd.AddCommand(initCmd)
}
