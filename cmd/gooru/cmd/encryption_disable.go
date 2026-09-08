package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/encryptionkeys"
	"gooru.local/internal/securekey"
	"gooru.local/internal/serve"
)

// migrateConfiguredStorageToPlaintext performs the one-time transition required
// when encryption.enabled is turned off while the configured database is still
// encrypted. Managed media is restored first and the database is converted
// last, so an interruption always leaves recoverable database state that can
// enumerate the remaining work on the next run. Already-restored media is
// accepted to make that retry path idempotent.
func migrateConfiguredStorageToPlaintext(cfg serve.Config, verbose bool) error {
	if cfg.Encryption.Enabled {
		return nil
	}
	if err := database.RecoverEncryptedDatabaseToPlaintextMigration(cfg.Database.Path); err != nil {
		return fmt.Errorf("recover interrupted protected-storage disable: %w", err)
	}
	_, err := os.Stat(cfg.Database.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect database before disabling protected storage: %w", err)
	}
	plain, err := database.IsPlaintextDatabase(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("inspect database before disabling protected storage: %w", err)
	}
	if plain {
		// This also completes a transition interrupted after the database swap but
		// before disposable encrypted derivatives were removed.
		if err := serve.CleanupProtectedMediaCache(cfg.Media); err != nil {
			return fmt.Errorf("clean protected media cache after disabling protected storage: %w", err)
		}
		return nil
	}

	master, err := loadEncryptionDisableRecoveryKey(cfg)
	if err != nil {
		return err
	}
	keys, err := encryptionkeys.Derive(master)
	if err != nil {
		return fmt.Errorf("derive protected-storage subkeys for disable transition: %w", err)
	}
	if err := ensureRecoveryDatabaseSubkey(cfg.Database.Path, master, keys.Database); err != nil {
		return err
	}

	client, err := gooru.NewWithOptions(cfg.Database.Path, verbose, gooru.OpenOptions{
		Database: gooru.DatabaseOpenOptions{EncryptionKey: keys.Database},
	})
	if err != nil {
		return fmt.Errorf("open protected database for disable transition: %w", err)
	}
	if err := restoreManagedUploadsToPlaintext(cfg, client, master, keys.Media); err != nil {
		_ = client.Close()
		return err
	}
	if err := client.Close(); err != nil {
		return fmt.Errorf("close protected database before plaintext replacement: %w", err)
	}

	// Derivatives are disposable, so remove the encrypted namespace rather than
	// decrypting it. Keeping this before the database swap preserves the invariant
	// that every fallible storage cleanup can be retried while the DB is encrypted.
	if err := serve.CleanupProtectedMediaCache(cfg.Media); err != nil {
		return fmt.Errorf("clean protected media cache before disabling protected storage: %w", err)
	}
	if err := database.MigrateEncryptedDatabaseToPlaintext(cfg.Database.Path, keys.Database); err != nil {
		return fmt.Errorf("restore database to plaintext storage: %w", err)
	}
	return nil
}

func loadEncryptionDisableRecoveryKey(cfg serve.Config) ([]byte, error) {
	keyFile := strings.TrimSpace(cfg.Encryption.KeyFile)
	_, hasEnvKey := os.LookupEnv(securekey.EnvKey)
	hasEnvKeyFile := strings.TrimSpace(os.Getenv(securekey.EnvKeyFile)) != ""
	if keyFile != "" && (hasEnvKey || hasEnvKeyFile) {
		return nil, fmt.Errorf("disable protected storage: configure exactly one recovery key source: encryption.key_file, %s, or %s", securekey.EnvKey, securekey.EnvKeyFile)
	}
	if keyFile != "" {
		key, err := securekey.Load(securekey.Source{File: keyFile})
		if err != nil {
			return nil, fmt.Errorf("disable protected storage: load original recovery key: %w", err)
		}
		return key, nil
	}
	key, configured, err := securekey.LoadProcess()
	if err != nil {
		return nil, fmt.Errorf("disable protected storage: load original recovery key: %w", err)
	}
	if !configured {
		return nil, fmt.Errorf("database is still encrypted while encryption.enabled is false: keep the original recovery key configured via encryption.key_file, %s, or %s for one successful startup so Gooru can restore managed storage to plaintext", securekey.EnvKey, securekey.EnvKeyFile)
	}
	return key, nil
}

// ensureRecoveryDatabaseSubkey accepts both current domain-separated databases
// and the historical master-key database format. The latter is normalized first
// so the remainder of the disable transition has one database key contract.
func ensureRecoveryDatabaseSubkey(path string, master, databaseKey []byte) error {
	derived, err := database.NewEncryptedStore(path, false, databaseKey)
	if err == nil {
		return derived.Close()
	}
	legacy, legacyErr := database.NewEncryptedStore(path, false, master)
	if legacyErr != nil {
		return fmt.Errorf("open protected database for disable transition: the configured recovery key does not match this database: %w", err)
	}
	if closeErr := legacy.Close(); closeErr != nil {
		return fmt.Errorf("close legacy-key database before disable transition: %w", closeErr)
	}
	if err := database.MigrateEncryptedDatabaseKey(path, master, databaseKey); err != nil {
		return fmt.Errorf("normalize legacy protected database key before disable transition: %w", err)
	}
	return nil
}

func restoreManagedUploadsToPlaintext(cfg serve.Config, client registeredFileLister, master, mediaKey []byte) error {
	files, err := client.GetAllFilesInfo()
	if err != nil {
		return fmt.Errorf("list registered files for protected-storage disable: %w", err)
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		path := file.Path
		if !serve.IsManagedUploadPath(cfg.Uploads.Targets, path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}

		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect managed upload before plaintext restoration: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing protected-storage disable of managed upload symlink %q", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing protected-storage disable of non-regular managed upload %q", path)
		}

		err = encryptedfile.DecryptFileInPlace(path, mediaKey)
		if err == nil || errors.Is(err, encryptedfile.ErrNotEncrypted) {
			continue
		}
		if errors.Is(err, encryptedfile.ErrAuthentication) {
			// Compatibility for protected media written before domain-separated
			// media keys landed. A successfully restored file remains plaintext and
			// is skipped on any subsequent interrupted-transition retry.
			if legacyErr := encryptedfile.DecryptFileInPlace(path, master); legacyErr == nil {
				continue
			} else {
				return fmt.Errorf("restore legacy-key managed upload to plaintext: %w", legacyErr)
			}
		}
		return fmt.Errorf("restore managed upload to plaintext storage: %w", err)
	}
	return nil
}
