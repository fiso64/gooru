package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
)

var serveFlags struct {
	configPath   string
	listen       string
	publicURL    string
	authToken    string
	printDefault bool
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Runs the Gooru HTTP API and frontend server.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dbPath, err := configuredDatabasePath()
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
			DatabasePath: databasePath,
		})
		if err != nil {
			return err
		}
		if err := ensureStorageEncryptionReady(cfg); err != nil {
			return err
		}

		logger := slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{Level: cfg.Logging.SlogLevel()}))
		previousLogger := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(previousLogger)

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
			sessionStore := serve.NewAuthStore(authStore.DB, cfg.Auth.SessionTTL)
			if err := sessionStore.CleanupExpiredSessions(ctx); err != nil {
				return fmt.Errorf("failed to clean up expired sessions: %w", err)
			}
			server.SetAuthStore(sessionStore)
		}

		logger.Info("gooru server starting",
			"url", startupURL(cfg),
			"auth_enabled", cfg.Auth.Enabled,
			"uploads_enabled", cfg.Uploads.Enabled,
		)
		logger.Debug("server runtime configuration",
			"logging_level", cfg.Logging.Level,
			"preview_size", cfg.Media.PreviewSize,
			"thumbnail_format", cfg.Media.ThumbnailFormat,
			"jobs_max_queued", cfg.Jobs.MaxQueued,
			"jobs_max_running", cfg.Jobs.MaxRunning,
		)
		err = server.ListenAndServe(ctx)
		if err == nil {
			logger.Info("gooru server stopped")
		}
		return err
	},
}

func startupURL(cfg serve.Config) string {
	if publicURL := strings.TrimSpace(cfg.Server.PublicURL); publicURL != "" {
		return strings.TrimRight(publicURL, "/")
	}

	host, port, err := net.SplitHostPort(strings.TrimSpace(cfg.Server.Listen))
	if err != nil {
		return "http://" + strings.TrimSpace(cfg.Server.Listen)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&serveFlags.configPath, "config", "", "Path to YAML server config")
	serveCmd.Flags().StringVar(&serveFlags.listen, "listen", "", "Override server.listen, for example 127.0.0.1:5678")
	serveCmd.Flags().StringVar(&serveFlags.publicURL, "public-url", "", "Override server.public_url")
	serveCmd.Flags().StringVar(&serveFlags.authToken, "auth-token", "", "Deprecated; DB-backed users replace token auth")
	serveCmd.Flags().BoolVar(&serveFlags.printDefault, "print-default-config", false, "Print the default YAML server config and exit")
}
