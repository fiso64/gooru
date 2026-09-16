package cmd

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
	"gooru.local/types"
)

var userCreateAdminFlags struct {
	username  string
	ifMissing bool
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
		created, err := createAdminWithPolicy(context.Background(), authStore, userCreateAdminFlags.username, password, userCreateAdminFlags.ifMissing)
		if err != nil {
			return err
		}
		username := strings.TrimSpace(userCreateAdminFlags.username)
		if !created {
			fmt.Fprintf(cmd.OutOrStdout(), "admin user %s already exists\n", username)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "created admin user %s\n", username)
		return nil
	},
}

func createAdminWithPolicy(ctx context.Context, authStore *serve.AuthStore, username, password string, ifMissing bool) (bool, error) {
	_, err := authStore.CreateAdmin(ctx, username, password)
	if errors.Is(err, serve.ErrDuplicateUsername) {
		if ifMissing {
			return false, nil
		}
		return false, fmt.Errorf("user %q already exists", username)
	}
	if err != nil {
		return false, err
	}
	return true, nil
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

	// The admin path uses the same configured storage composition as serve and
	// ordinary CLI commands. This keeps plaintext/encrypted SQLite selection and
	// key handling out of feature code while still allowing create-admin to
	// bootstrap a fresh database before a full Gooru client can be opened.
	store, err := openConfiguredAuthStore(cfg, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to open configured admin database: %w", err)
	}
	closeOnError := func(err error) (*database.Store, error) {
		_ = store.Close()
		return nil, err
	}
	if err := database.RunMigrations(store.DB); err != nil {
		return closeOnError(fmt.Errorf("failed to migrate configured admin database: %w", err))
	}
	if err := database.SecureDBFiles(dbPath); err != nil {
		return closeOnError(err)
	}
	if _, err := store.GetHashingStrategy(); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return closeOnError(fmt.Errorf("failed to read hashing strategy: %w", err))
		}
		if err := store.SetHashingStrategy(types.StrategyPartial); err != nil {
			return closeOnError(fmt.Errorf("failed to save default hashing strategy: %w", err))
		}
		if err := database.SecureDBFiles(dbPath); err != nil {
			return closeOnError(err)
		}
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
	userCreateAdminCmd.Flags().BoolVar(&userCreateAdminFlags.ifMissing, "if-missing", false, "Succeed without changing the account when the username already exists")
}
