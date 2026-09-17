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

var initFlags struct {
	hashingStrategy string
	ifMissing       bool
}

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initializes a new Gooru database.",
	Long: `Initializes a new Gooru database at the configured location.

The default remains ~/.config/gooru/gooru.db. Use the persistent --database flag
when initializing an independent instance database.

This is the first command you must run. It will prompt you to choose a hashing
strategy for the new database unless --hashing-strategy is provided. This choice
is permanent and cannot be changed later.

Use --if-missing for first-boot automation. When a database file already exists,
that mode leaves it completely unchanged instead of reconciling its strategy.

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

		if _, err := os.Stat(dbPath); err == nil {
			if initFlags.ifMissing {
				fmt.Fprintf(cmd.OutOrStdout(), "database already exists at %s; leaving it unchanged\n", dbPath)
				return nil
			}
			return fmt.Errorf("database file already exists at: %s\nTo start over, delete this file and run 'init' again", dbPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to check for existing database: %w", err)
		}

		strategy, err := initHashingStrategy(cmd)
		if err != nil {
			return err
		}

		created, err := initializeDatabaseIfNeeded(dbPath, strategy, initFlags.ifMissing, verbose)
		if err != nil {
			return err
		}
		if !created {
			fmt.Fprintf(cmd.OutOrStdout(), "database already exists at %s; leaving it unchanged\n", dbPath)
			return nil
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Database initialized successfully at %s with '%s' hashing.\n", dbPath, strategy)
		return nil
	},
}

func initHashingStrategy(cmd *cobra.Command) (types.HashingStrategy, error) {
	if value := strings.TrimSpace(strings.ToLower(initFlags.hashingStrategy)); value != "" {
		switch value {
		case string(types.StrategyPartial):
			return types.StrategyPartial, nil
		case string(types.StrategyFull):
			return types.StrategyFull, nil
		default:
			return "", fmt.Errorf("invalid --hashing-strategy %q: must be partial or full", initFlags.hashingStrategy)
		}
	}
	if initFlags.ifMissing {
		return "", fmt.Errorf("--hashing-strategy is required with --if-missing when creating a database")
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Choose a hashing strategy for the new database.")
	fmt.Fprintln(cmd.OutOrStdout(), "This choice is permanent and affects performance vs. reliability trade-offs.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "1) Partial Hashing (Recommended, Faster for large files)")
	fmt.Fprintln(cmd.OutOrStdout(), "   - Hashes file size + strategic chunks. Ideal for media libraries.")
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "2) Full Hashing (Most Reliable, Slower for large files)")
	fmt.Fprintln(cmd.OutOrStdout(), "   - Hashes the entire file content. Best for critical documents.")
	fmt.Fprintln(cmd.OutOrStdout())

	reader := bufio.NewReader(cmd.InOrStdin())
	for {
		fmt.Fprint(cmd.OutOrStdout(), "Enter your choice (1 or 2): ")
		input, readErr := reader.ReadString('\n')
		if readErr != nil {
			return "", fmt.Errorf("failed to read hashing strategy: %w", readErr)
		}
		switch strings.TrimSpace(input) {
		case "1":
			return types.StrategyPartial, nil
		case "2":
			return types.StrategyFull, nil
		default:
			fmt.Fprintln(cmd.OutOrStdout(), "Invalid choice. Please enter 1 or 2.")
		}
	}
}

func initializeDatabaseIfNeeded(dbPath string, strategy types.HashingStrategy, ifMissing bool, verbose bool) (bool, error) {
	if _, err := os.Stat(dbPath); err == nil {
		if ifMissing {
			return false, nil
		}
		return false, fmt.Errorf("database file already exists at: %s\nTo start over, delete this file and run 'init' again", dbPath)
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to check for existing database: %w", err)
	}

	if err := ensureDatabaseParent(dbPath); err != nil {
		return false, fmt.Errorf("failed to create database directory: %w", err)
	}
	if err := gooru.Init(dbPath, strategy, verbose); err != nil {
		return false, fmt.Errorf("database initialization failed: %w", err)
	}
	return true, nil
}

func ensureDatabaseParent(dbPath string) error {
	return os.MkdirAll(filepath.Dir(dbPath), 0700)
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&initFlags.hashingStrategy, "hashing-strategy", "", "Hashing strategy for a new database: partial or full (skips the interactive prompt)")
	initCmd.Flags().BoolVar(&initFlags.ifMissing, "if-missing", false, "Initialize only when the database file is missing; never change an existing database")
}
