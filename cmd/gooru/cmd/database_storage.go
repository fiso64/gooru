package cmd

import (
	"fmt"
	"os"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/encryptionkeys"
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
		return fmt.Errorf("database remains encrypted after protected-storage disable preparation")
	}
	return nil
}

func configuredEncryptionKeys(cfg serve.Config) (encryptionkeys.Keys, error) {
	if !cfg.Encryption.Enabled {
		return encryptionkeys.Keys{}, nil
	}
	keys, err := encryptionkeys.Derive(cfg.Encryption.Key)
	if err != nil {
		return encryptionkeys.Keys{}, fmt.Errorf("derive protected-storage subkeys: %w", err)
	}
	return keys, nil
}

func recoverDisabledDatabaseKeyMigration(cfg serve.Config) error {
	if cfg.Encryption.Enabled {
		return nil
	}
	// A crash can stage the historical-key database away from the canonical path
	// immediately before the user disables protected storage. Restoring that
	// deterministic rollback copy does not require a key; any installed rekeyed
	// database is left untouched until a keyed recovery pass can verify it.
	if err := database.RecoverEncryptedDatabaseKeyMigration(cfg.Database.Path, nil); err != nil {
		return fmt.Errorf("recover interrupted database subkey migration before disabling protected storage: %w", err)
	}
	return nil
}

// ensureConfiguredDatabaseKey performs the one-time compatibility migration
// from the historical master-key database format to the database-domain subkey.
// A plaintext database is left for the normal protected-mode migration path.
func ensureConfiguredDatabaseKey(cfg serve.Config, databaseKey []byte) error {
	if !cfg.Encryption.Enabled {
		return nil
	}
	if err := database.RecoverEncryptedDatabaseKeyMigration(cfg.Database.Path, databaseKey); err != nil {
		return fmt.Errorf("recover interrupted database subkey migration: %w", err)
	}
	plain, err := database.IsPlaintextDatabase(cfg.Database.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect database before subkey migration: %w", err)
	}
	if plain {
		return nil
	}

	derived, err := database.NewEncryptedStore(cfg.Database.Path, false, databaseKey)
	if err == nil {
		return derived.Close()
	}

	legacy, legacyErr := database.NewEncryptedStore(cfg.Database.Path, false, cfg.Encryption.Key)
	if legacyErr != nil {
		return nil // let the normal open path return the actionable wrong-key error
	}
	if closeErr := legacy.Close(); closeErr != nil {
		return fmt.Errorf("close legacy-key database before subkey migration: %w", closeErr)
	}
	if err := database.MigrateEncryptedDatabaseKey(cfg.Database.Path, cfg.Encryption.Key, databaseKey); err != nil {
		return fmt.Errorf("migrate protected database to database-domain key: %w", err)
	}
	return nil
}

func openConfiguredClient(cfg serve.Config, verbose bool) (*gooru.Client, error) {
	if err := recoverDisabledDatabaseKeyMigration(cfg); err != nil {
		return nil, err
	}
	if err := migrateConfiguredStorageToPlaintext(cfg, verbose); err != nil {
		return nil, err
	}
	if err := validateConfiguredEncryptionLifecycle(cfg); err != nil {
		return nil, err
	}

	options := gooru.OpenOptions{}
	if cfg.Encryption.Enabled {
		keys, err := configuredEncryptionKeys(cfg)
		if err != nil {
			return nil, err
		}
		if err := ensureConfiguredDatabaseKey(cfg, keys.Database); err != nil {
			return nil, err
		}
		options.Database.EncryptionKey = keys.Database
		options.Database.MigratePlaintext = true
		options.Content.EncryptionKey = keys.Media
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
	if err := recoverDisabledDatabaseKeyMigration(cfg); err != nil {
		return nil, err
	}
	if err := migrateConfiguredStorageToPlaintext(cfg, verbose); err != nil {
		return nil, err
	}
	if err := validateConfiguredEncryptionLifecycle(cfg); err != nil {
		return nil, err
	}
	if cfg.Encryption.Enabled {
		keys, err := configuredEncryptionKeys(cfg)
		if err != nil {
			return nil, err
		}
		if err := ensureConfiguredDatabaseKey(cfg, keys.Database); err != nil {
			return nil, err
		}
		store, err := database.NewEncryptedStore(cfg.Database.Path, verbose, keys.Database)
		if err != nil {
			return nil, fmt.Errorf("open protected auth database with configured key: verify the recovery-critical encryption key matches this database; automatic key rotation/re-key is not supported: %w", err)
		}
		return store, nil
	}
	return database.NewStore(cfg.Database.Path, verbose)
}
