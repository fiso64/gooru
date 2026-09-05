package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

var userCreateAdminFlags struct {
	username string
}

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manages DB-backed users.",
}

var userCreateAdminCmd = &cobra.Command{
	Use:   "create-admin",
	Short: "Creates an admin user in the configured Gooru database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(userCreateAdminFlags.username) == "" {
			return errors.New("--username is required")
		}
		dbPath, err := configuredDatabasePath()
		if err != nil {
			return fmt.Errorf("failed to get db path: %w", err)
		}
		cfg, err := serve.LoadConfig(configPath, dbPath, serve.Overrides{DatabasePath: databasePath})
		if err != nil {
			return err
		}
		store, err := prepareAdminDatabase(cfg, verbose)
		if err != nil {
			return err
		}
		defer store.Close()
		password := strings.TrimSpace(os.Getenv("GOORU_ADMIN_PASSWORD"))
		if password == "" {
			password, err = promptPassword(cmd)
			if err != nil {
				return err
			}
		}
		authStore := serve.NewAuthStore(store.DB, cfg.Auth.SessionTTL)
		user, err := authStore.CreateAdmin(context.Background(), userCreateAdminFlags.username, password)
		if errors.Is(err, serve.ErrDuplicateUsername) {
			return fmt.Errorf("user %q already exists", userCreateAdminFlags.username)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created admin user %s\n", user.Username)
		return nil
	},
}

func prepareAdminDatabase(cfg serve.Config, verbose bool) (*database.Store, error) {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		return nil, errors.New("database.path is required")
	}
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		if _, err := os.Stat(dir); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to inspect database directory: %w", err)
			}
			if err := os.MkdirAll(dir, 0700); err != nil {
				return nil, fmt.Errorf("failed to create database directory: %w", err)
			}
			if err := os.Chmod(dir, 0700); err != nil {
				return nil, fmt.Errorf("failed to secure database directory: %w", err)
			}
		}
	}

	// Initialize an absent/uninitialized database through the ordinary core
	// initializer, then immediately reopen it through the configured storage
	// policy. Protected mode therefore performs the same explicit plaintext ->
	// encrypted migration as serve and normal CLI commands rather than letting
	// this admin path choose a database implementation itself.
	client, err := openConfiguredClient(cfg, verbose)
	if errors.Is(err, gooru.ErrDBUninitialized) {
		if err := gooru.Init(dbPath, types.StrategyPartial, verbose); err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		client, err = openConfiguredClient(cfg, verbose)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to prepare configured database: %w", err)
	}
	if err := client.Close(); err != nil {
		return nil, fmt.Errorf("failed to close configured database client: %w", err)
	}

	store, err := openConfiguredAuthStore(cfg, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to open configured admin database: %w", err)
	}
	return store, nil
}

func promptPassword(cmd *cobra.Command) (string, error) {
	fd := int(os.Stdin.Fd())
	var password string
	var confirm string
	if term.IsTerminal(fd) {
		var err error
		password, err = readTerminalSecret(cmd, fd, "Password: ")
		if err != nil {
			return "", err
		}
		confirm, err = readTerminalSecret(cmd, fd, "Confirm password: ")
		if err != nil {
			return "", err
		}
	} else {
		reader := bufio.NewReader(os.Stdin)
		var err error
		password, err = readLineSecret(cmd, reader, "Password: ")
		if err != nil {
			return "", err
		}
		confirm, err = readLineSecret(cmd, reader, "Confirm password: ")
		if err != nil {
			return "", err
		}
	}
	if password != confirm {
		return "", errors.New("passwords do not match")
	}
	if err := serve.ValidatePassword(password); err != nil {
		return "", err
	}
	return password, nil
}

func readTerminalSecret(cmd *cobra.Command, fd int, prompt string) (string, error) {
	fmt.Fprint(cmd.ErrOrStderr(), prompt)
	data, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.ErrOrStderr())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readLineSecret(cmd *cobra.Command, reader *bufio.Reader, prompt string) (string, error) {
	fmt.Fprint(cmd.ErrOrStderr(), prompt)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(value, "\r\n"), nil
}

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userCreateAdminCmd)
	userCreateAdminCmd.Flags().StringVar(&userCreateAdminFlags.username, "username", "", "Admin username")
}
