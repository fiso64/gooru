package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gooru.local/internal/serve"
)

var userSetPasswordFlags struct {
	username string
}

var userSetPasswordCmd = &cobra.Command{
	Use:   "set-password",
	Short: "Sets a user's password in the configured Gooru database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		username := strings.TrimSpace(userSetPasswordFlags.username)
		if username == "" {
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

func init() {
	userCmd.AddCommand(userSetPasswordCmd)
	userSetPasswordCmd.Flags().StringVar(&userSetPasswordFlags.username, "username", "", "Username whose password should be replaced")
}
