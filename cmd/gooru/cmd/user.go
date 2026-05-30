package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gooru.local/cmd/gooru/config"
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
)

var userCreateAdminFlags struct {
	configPath string
	username   string
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
		dbPath, err := config.GetDBPath()
		if err != nil {
			return fmt.Errorf("failed to get db path: %w", err)
		}
		cfg, err := serve.LoadConfig(userCreateAdminFlags.configPath, dbPath, serve.Overrides{})
		if err != nil {
			return err
		}
		client, err := gooru.New(cfg.Database.Path, verbose)
		if err != nil {
			if errors.Is(err, gooru.ErrDBUninitialized) {
				return fmt.Errorf("database not initialized. Please run 'gooru init' first")
			}
			return fmt.Errorf("failed to migrate database: %w", err)
		}
		_ = client.Close()
		store, err := database.NewStore(cfg.Database.Path, verbose)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
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
	if err := serve.ValidatePasswordStrength(password); err != nil {
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
	userCreateAdminCmd.Flags().StringVar(&userCreateAdminFlags.configPath, "config", "", "Path to YAML server config")
	userCreateAdminCmd.Flags().StringVar(&userCreateAdminFlags.username, "username", "", "Admin username")
}
