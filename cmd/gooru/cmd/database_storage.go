package cmd

import (
	"fmt"
	"os"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
)

func validateConfiguredEncryptionLifecycle(cfg serve.Config) error {
	_, err := os.Stat(cfg.Database.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect database before applying encryption configuration: %w", err)
	}

	plain, err := database.IsPlaintextDatabase(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("inspect database encryption state: %w", err)
	}
	if !cfg.Encryption.Enabled && !plain {
		return fmt.Errorf("database appears encrypted but encryption.enabled is false: disabling protected storage in place is not supported; restore encryption.enabled and the original recovery key before accessing this database")
	}
	return nil
}

func openConfiguredClient(cfg serve.Config, verbose bool) (*gooru.Client, error) {
	if err := validateConfiguredEncryptionLifecycle(cfg); err != nil {
		return nil, err
	}

	options := gooru.OpenOptions{}
	if cfg.Encryption.Enabled {
		options.Database.EncryptionKey = cfg.Encryption.Key
		options.Database.MigratePlaintext = true
		options.Content.EncryptionKey = cfg.Encryption.Key
		options.Content.ProtectedRoots = make([]string, 0, len(cfg.Uploads.Targets))
		for _, target := range cfg.Uploads.Targets {
			options.Content.ProtectedRoots = append(options.Content.ProtectedRoots, target.Path)
		}
	}
	client, err := gooru.NewWithOptions(cfg.Database.Path, verbose, options)
	if err != nil && cfg.Encryption.Enabled {
		plain, inspectErr := database.IsPlaintextDatabase(cfg.Database.Path)
		if inspectErr == nil && !plain {
			return nil, fmt.Errorf("open protected database with configured key: verify the recovery-critical encryption key matches this database; automatic key rotation/re-key is not supported: %w", err)
		}
	}
	return client, err
}

func openConfiguredAuthStore(cfg serve.Config, verbose bool) (*database.Store, error) {
	if err := validateConfiguredEncryptionLifecycle(cfg); err != nil {
		return nil, err
	}
	if cfg.Encryption.Enabled {
		store, err := database.NewEncryptedStore(cfg.Database.Path, verbose, cfg.Encryption.Key)
		if err != nil {
			return nil, fmt.Errorf("open protected auth database with configured key: verify the recovery-critical encryption key matches this database; automatic key rotation/re-key is not supported: %w", err)
		}
		return store, nil
	}
	return database.NewStore(cfg.Database.Path, verbose)
}
