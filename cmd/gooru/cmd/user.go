package cmd

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
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
		password, err := adminPassword(cmd)
		if err != nil {
			return err
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
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("database not initialized at %q; run 'gooru init' first", dbPath)
		}
		return nil, fmt.Errorf("failed to inspect configured admin database: %w", err)
	}

	// User management must not perform database initialization. Open the existing
	// configured storage composition, then verify the initialization invariants
	// established by `gooru init` before making any user changes.
	store, err := openConfiguredAuthStore(cfg, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to open configured admin database: %w", err)
	}
	closeOnError := func(err error) (*database.Store, error) {
		_ = store.Close()
		return nil, err
	}
	initialized, err := store.IsInitialized()
	if err != nil {
		return closeOnError(fmt.Errorf("failed to inspect database initialization state: %w", err))
	}
	if !initialized {
		return closeOnError(errors.New("database not initialized; run 'gooru init' first"))
	}
	if _, err := store.GetHashingStrategy(); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return closeOnError(errors.New("database not initialized: hashing strategy is missing; run 'gooru init' first"))
		}
		return closeOnError(fmt.Errorf("failed to read hashing strategy: %w", err))
	}
	return store, nil
}

func adminPassword(cmd *cobra.Command) (string, error) {
	if password, ok := os.LookupEnv("GOORU_ADMIN_PASSWORD"); ok {
		if err := serve.ValidatePassword(password); err != nil {
			return "", err
		}
		return password, nil
	}
	return promptPassword(cmd)
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
