package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/internal/serve"
)

var userSetPasswordFlags struct{ username string }
var userReconcileAdminFlags struct {
	username string
	userID   string
}

func configuredAuthStore() (*serve.AuthStore, func() error, error) {
	dbPath, err := configuredDatabasePath()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get db path: %w", err)
	}
	cfg, err := serve.LoadConfig(configPath, dbPath, serve.Overrides{DatabasePath: databasePath})
	if err != nil {
		return nil, nil, err
	}
	store, err := prepareAdminDatabase(cfg, verbose)
	if err != nil {
		return nil, nil, err
	}
	return serve.NewAuthStore(store.DB, cfg.Auth.SessionTTL), store.Close, nil
}

var userSetPasswordCmd = &cobra.Command{
	Use:   "set-password",
	Short: "Sets a user's password in the configured Gooru database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		username := strings.TrimSpace(userSetPasswordFlags.username)
		if username == "" {
			return errors.New("--username is required")
		}
		authStore, closeStore, err := configuredAuthStore()
		if err != nil {
			return err
		}
		defer closeStore()
		password, err := adminPassword(cmd)
		if err != nil {
			return err
		}
		user, err := authStore.SetPasswordByUsername(context.Background(), username, password)
		if errors.Is(err, serve.ErrUserNotFound) {
			return fmt.Errorf("user %q does not exist", username)
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "updated password for user %s\n", user.Username)
		return nil
	},
}

var userReconcileAdminCmd = &cobra.Command{
	Use:   "reconcile-admin",
	Short: "Reconciles one declaratively managed administrator.",
	RunE: func(cmd *cobra.Command, args []string) error {
		username := strings.TrimSpace(userReconcileAdminFlags.username)
		if username == "" {
			return errors.New("--username is required")
		}
		authStore, closeStore, err := configuredAuthStore()
		if err != nil {
			return err
		}
		defer closeStore()
		password, err := adminPassword(cmd)
		if err != nil {
			return err
		}
		user, err := authStore.ReconcileAdmin(context.Background(), userReconcileAdminFlags.userID, username, password)
		if errors.Is(err, serve.ErrUserNotFound) {
			return fmt.Errorf("managed user id %q does not exist", strings.TrimSpace(userReconcileAdminFlags.userID))
		}
		if errors.Is(err, serve.ErrDuplicateUsername) {
			return fmt.Errorf("username %q is already used by another user", username)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), user.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(userCmd)
	userCmd.AddCommand(userCreateAdminCmd)
	userCreateAdminCmd.Flags().StringVar(&userCreateAdminFlags.username, "username", "", "Admin username")
	userCreateAdminCmd.Flags().BoolVar(&userCreateAdminFlags.ifMissing, "if-missing", false, "Succeed without changing the account when the username already exists")
	userCmd.AddCommand(userSetPasswordCmd)
	userSetPasswordCmd.Flags().StringVar(&userSetPasswordFlags.username, "username", "", "Username whose password should be replaced")
	userCmd.AddCommand(userReconcileAdminCmd)
	userReconcileAdminCmd.Flags().StringVar(&userReconcileAdminFlags.username, "username", "", "Desired admin username")
	userReconcileAdminCmd.Flags().StringVar(&userReconcileAdminFlags.userID, "user-id", "", "Previously persisted stable Gooru user ID")
}
