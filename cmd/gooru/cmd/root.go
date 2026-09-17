/*
Copyright © 2025 Your Name
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/config"
	"gooru.local/cmd/gooru/display"
	"gooru.local/gooru"
	"gooru.local/internal/serve"
)

var (
	svc          *gooru.Client
	verbose      bool
	databasePath string
	configPath   string
	rootCmd      = &cobra.Command{
		Use:   "gooru",
		Short: "A blazing-fast local file tagger.",
		Long:  `Gooru is a CLI tool for tagging local files. It uses content hashing to track files, so tags are stable across renames and moves.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !commandUsesConfiguredClient(cmd) {
				return nil
			}

			cfg, err := loadCommandConfig(serve.Overrides{DatabasePath: databasePath})
			if err != nil {
				return err
			}

			svc, err = openConfiguredClient(cfg, verbose)
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
				svc = nil
			}
		},
	}
)

func commandUsesConfiguredClient(cmd *cobra.Command) bool {
	// These commands either create/open storage through their own composition
	// boundary or do not need the application client at all. In particular, user
	// management must not run the full client pre-run: that path applies schema
	// and protected-storage migrations before the dedicated auth-store checks.
	if cmd.Name() == "init" || cmd.Name() == "serve" || cmd.Name() == "version" {
		return false
	}
	path := cmd.CommandPath()
	return path != "gooru user" && !strings.HasPrefix(path, "gooru user ")
}

func defaultDatabasePath() (string, error) {
	return config.GetDBPath()
}

func loadCommandConfig(overrides serve.Overrides) (serve.Config, error) {
	dbPath, err := defaultDatabasePath()
	if err != nil {
		return serve.Config{}, fmt.Errorf("failed to get db path: %w", err)
	}
	cfg, err := serve.LoadConfig(configPath, dbPath, overrides)
	if err != nil {
		return serve.Config{}, err
	}
	return cfg, nil
}

func configuredDatabasePath() (string, error) {
	if path := strings.TrimSpace(databasePath); path != "" {
		return path, nil
	}
	if strings.TrimSpace(configPath) == "" {
		return defaultDatabasePath()
	}
	cfg, err := loadCommandConfig(serve.Overrides{})
	if err != nil {
		return "", err
	}
	return cfg.Database.Path, nil
}

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
	rootCmd.PersistentFlags().StringVar(&databasePath, "database", "", "Path to the Gooru database (overrides the default and database.path in server config)")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Path to YAML server config (also configures protected storage for CLI commands)")
}
