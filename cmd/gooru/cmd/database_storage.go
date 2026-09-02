package cmd

import (
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
)

func openConfiguredClient(cfg serve.Config, verbose bool) (*gooru.Client, error) {
	options := gooru.DatabaseOpenOptions{}
	if cfg.Encryption.Enabled {
		options.EncryptionKey = cfg.Encryption.Key
		options.MigratePlaintext = true
	}
	return gooru.NewWithDatabaseOptions(cfg.Database.Path, verbose, options)
}

func openConfiguredAuthStore(cfg serve.Config, verbose bool) (*database.Store, error) {
	if cfg.Encryption.Enabled {
		return database.NewEncryptedStore(cfg.Database.Path, verbose, cfg.Encryption.Key)
	}
	return database.NewStore(cfg.Database.Path, verbose)
}
