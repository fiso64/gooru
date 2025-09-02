/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/config"
	"gooru.local/cmd/gooru/display"
	"gooru.local/gooru"
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
	// By setting SilenceErrors and SilenceUsage to true, we can handle error
	// and usage printing ourselves. This allows for custom colored output and
	// prevents the usage string from printing on every error.
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	err := rootCmd.Execute()
	if err != nil {
		// Special case for the 'exists' command to return a non-zero exit code
		// for a "false" result, which is not a true error.
		if err == ErrExitCode1 {
			os.Exit(1)
		}
		// Use our custom Errorf function to print the error in red.
		// Cobra's default behavior is to print "Error: <err.Error()>" to stderr.
		// We replicate this but with color.
		display.Errorf("%v", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose logging, including SQL statements")
}