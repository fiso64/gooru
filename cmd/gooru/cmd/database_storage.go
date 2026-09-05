package cmd

import (
	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/serve"
)

func openConfiguredClient(cfg serve.Config, verbose bool) (*gooru.Client, error) {
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
	return gooru.NewWithOptions(cfg.Database.Path, verbose, options)
}

func openConfiguredAuthStore(cfg serve.Config, verbose bool) (*database.Store, error) {
	if cfg.Encryption.Enabled {
		return database.NewEncryptedStore(cfg.Database.Path, verbose, cfg.Encryption.Key)
	}
	return database.NewStore(cfg.Database.Path, verbose)
}
