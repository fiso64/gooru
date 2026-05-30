package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"gooru.local/cmd/gooru/config"
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
)

var serveFlags struct {
	configPath   string
	listen       string
	publicURL    string
	authToken    string
	databasePath string
	printDefault bool
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Runs the Gooru HTTP API and frontend server.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := config.GetDBPath()
		if err != nil {
			return fmt.Errorf("failed to get db path: %w", err)
		}
		if serveFlags.printDefault {
			data, err := serve.DefaultYAML(dbPath)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		}
		cfg, err := serve.LoadConfig(serveFlags.configPath, dbPath, serve.Overrides{
			Listen:       serveFlags.listen,
			PublicURL:    serveFlags.publicURL,
			AuthToken:    serveFlags.authToken,
			DatabasePath: serveFlags.databasePath,
		})
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		client, err := gooru.New(cfg.Database.Path, verbose)
		if err != nil {
			if err == gooru.ErrDBUninitialized {
				return fmt.Errorf("database not initialized. Please run 'gooru init' first")
			}
			return fmt.Errorf("failed to initialize gooru client: %w", err)
		}
		defer client.Close()
		server := serve.NewServerWithLibrary(cfg, serve.NewGooruLibrary(client, verbose))
		if cfg.Auth.Enabled {
			authStore, err := database.NewStore(cfg.Database.Path, verbose)
			if err != nil {
				return fmt.Errorf("failed to initialize auth store: %w", err)
			}
			defer authStore.Close()
			server.SetAuthStore(serve.NewAuthStore(authStore.DB, cfg.Auth.SessionTTL))
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "serving gooru on http://%s\n", cfg.Server.Listen)
		return server.ListenAndServe(ctx)
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&serveFlags.configPath, "config", "", "Path to YAML server config")
	serveCmd.Flags().StringVar(&serveFlags.listen, "listen", "", "Override server.listen, for example 127.0.0.1:5678")
	serveCmd.Flags().StringVar(&serveFlags.publicURL, "public-url", "", "Override server.public_url")
	serveCmd.Flags().StringVar(&serveFlags.authToken, "auth-token", "", "Deprecated; DB-backed users replace token auth")
	serveCmd.Flags().StringVar(&serveFlags.databasePath, "database", "", "Override database.path")
	serveCmd.Flags().BoolVar(&serveFlags.printDefault, "print-default-config", false, "Print the default YAML server config and exit")
}
