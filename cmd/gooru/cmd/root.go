/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"os"

	"gooru.local/gooru"
	"gooru.local/gooru/cmd/gooru/config"
	"github.com/spf13/cobra"
)

var (
	svc     *gooru.Client
	verbose bool
	rootCmd = &cobra.Command{
		Use:   "gooru",
		Short: "A blazing-fast local file tagger.",
		Long:  `Gooru is a CLI tool for tagging local files. It uses content hashing to track files, so tags are stable across renames and moves.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// The 'init' command is special: it creates the DB and must run before a client can be initialized.
			if cmd.Name() == "init" {
				return nil
			}

			dbPath, err := config.GetDBPath()
			if err != nil {
				return fmt.Errorf("failed to get db path: %w", err)
			}

			svc, err = gooru.New(dbPath, verbose)
			if err != nil {
				if err == gooru.ErrDBUninitialized {
					return fmt.Errorf("database not initialized. Please run 'gooru init' first")
				}
				return fmt.Errorf("failed to initialize gooru client: %w", err)
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if svc != nil {
				svc.Close()
			}
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose logging, including SQL statements")
}