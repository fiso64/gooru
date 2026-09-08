package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	plaintextDisableTargetSuffix = ".gooru-disable-plaintext"
	encryptedDisableBackupSuffix = ".gooru-disable-encrypted-backup"
)

// RecoverEncryptedDatabaseToPlaintextMigration repairs the durable swap state
// left by an interrupted protected-storage disable. The filenames are
// deterministic so a later process can distinguish Gooru-owned transition
// artifacts from arbitrary sibling files.
func RecoverEncryptedDatabaseToPlaintextMigration(path string) error {
	backupPath := path + encryptedDisableBackupSuffix
	plaintextPath := path + plaintextDisableTargetSuffix

	_, pathErr := os.Stat(path)
	_, backupErr := os.Stat(backupPath)
	switch {
	case os.IsNotExist(pathErr) && backupErr == nil:
		// The process died after staging the encrypted source but before installing
		// the verified plaintext target. Restore the encrypted source and let the
		// normal migration retry from a known-good state.
		if err := os.Rename(backupPath, path); err != nil {
			return fmt.Errorf("restore encrypted database after interrupted disable: %w", err)
		}
		cleanupSQLiteFiles(plaintextPath)
		return syncDatabaseDir(filepath.Dir(path))
	case pathErr != nil:
		if os.IsNotExist(pathErr) && os.IsNotExist(backupErr) {
			return nil
		}
		return fmt.Errorf("inspect database during disable recovery: %w", pathErr)
	case backupErr != nil && !os.IsNotExist(backupErr):
		return fmt.Errorf("inspect encrypted disable backup: %w", backupErr)
	}

	plain, err := IsPlaintextDatabase(path)
	if err != nil {
		return err
	}
	if plain && backupErr == nil {
		// The process died after the plaintext database became canonical but before
		// removing its encrypted rollback copy. Verify the installed DB before
		// deleting that recovery artifact.
		store, err := NewStore(path, false)
		if err != nil {
			return fmt.Errorf("reopen plaintext database during disable recovery: %w", err)
		}
		verifyErr := checkDatabaseIntegrity(store.DB)
		closeErr := store.Close()
		if verifyErr != nil {
			return fmt.Errorf("verify plaintext database during disable recovery: %w", verifyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close plaintext database during disable recovery: %w", closeErr)
		}
		if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove completed encrypted disable backup: %w", err)
		}
		cleanupSQLiteSidecars(backupPath)
		cleanupSQLiteFiles(plaintextPath)
		return syncDatabaseDir(filepath.Dir(path))
	}

	// A stale plaintext target can exist if the process stopped before staging
	// the encrypted source. It is never authoritative while the canonical path is
	// still encrypted, so it is safe to discard and rebuild.
	cleanupSQLiteFiles(plaintextPath)
	if backupErr == nil {
		return fmt.Errorf("ambiguous protected-storage disable state: both encrypted database and encrypted rollback backup exist")
	}
	return nil
}

// MigrateEncryptedDatabaseToPlaintext atomically rewrites an existing
// page-encrypted SQLite database into an ordinary plaintext SQLite database at
// the same path. The plaintext copy is built and integrity-checked beside the
// encrypted source; the encrypted source remains the rollback copy until the
// installed plaintext database reopens and passes integrity_check.
//
// The caller must ensure no other process is using the database during this
// one-time migration.
func MigrateEncryptedDatabaseToPlaintext(path string, key []byte) error {
	if len(key) != encryptedDatabaseKeySize {
		return ErrInvalidDatabaseEncryptionKey
	}
	if err := RecoverEncryptedDatabaseToPlaintextMigration(path); err != nil {
		return err
	}
	plain, err := IsPlaintextDatabase(path)
	if err != nil {
		return err
	}
	if plain {
		return nil
	}

	dir := filepath.Dir(path)
	plaintextPath := path + plaintextDisableTargetSuffix
	backupPath := path + encryptedDisableBackupSuffix
	cleanupSQLiteFiles(plaintextPath)
	defer cleanupSQLiteFiles(plaintextPath)

	source, err := NewEncryptedStore(path, false, key)
	if err != nil {
		return fmt.Errorf("open encrypted database for plaintext migration: %w", err)
	}
	sourceOpen := true
	defer func() {
		if sourceOpen {
			_ = source.Close()
		}
	}()
	if _, err := source.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint encrypted database: %w", err)
	}

	destination, err := NewStore(plaintextPath, false)
	if err != nil {
		return fmt.Errorf("create plaintext migration target: %w", err)
	}
	destinationOpen := true
	defer func() {
		if destinationOpen {
			_ = destination.Close()
		}
	}()
	if err := backupDatabase(context.Background(), source.DB, destination.DB); err != nil {
		return fmt.Errorf("copy database into plaintext target: %w", err)
	}
	if _, err := destination.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint plaintext migration target: %w", err)
	}
	if err := checkDatabaseIntegrity(destination.DB); err != nil {
		return fmt.Errorf("verify plaintext migration target: %w", err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close plaintext migration target: %w", err)
	}
	destinationOpen = false
	if err := source.Close(); err != nil {
		return fmt.Errorf("close encrypted migration source: %w", err)
	}
	sourceOpen = false

	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("stage encrypted database for replacement: %w", err)
	}
	cleanupSQLiteSidecars(path)
	if err := syncDatabaseDir(dir); err != nil {
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("sync staged encrypted database replacement: %w", err)
	}
	if err := os.Rename(plaintextPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("install plaintext database: %w", err)
	}
	if err := syncDatabaseDir(dir); err != nil {
		return fmt.Errorf("sync installed plaintext database: %w", err)
	}

	verified, err := NewStore(path, false)
	if err != nil {
		_ = os.Remove(path)
		_ = os.Rename(backupPath, path)
		_ = syncDatabaseDir(dir)
		return fmt.Errorf("reopen plaintext database after migration: %w", err)
	}
	verifyErr := checkDatabaseIntegrity(verified.DB)
	closeErr := verified.Close()
	if verifyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		_ = os.Rename(backupPath, path)
		_ = syncDatabaseDir(dir)
		if verifyErr != nil {
			return fmt.Errorf("verify installed plaintext database: %w", verifyErr)
		}
		return fmt.Errorf("close installed plaintext database: %w", closeErr)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove encrypted migration backup: %w", err)
	}
	cleanupSQLiteSidecars(backupPath)
	if err := syncDatabaseDir(dir); err != nil {
		return fmt.Errorf("sync completed plaintext database migration: %w", err)
	}
	return SecureDBFiles(path)
}

func syncDatabaseDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync database directory: %w", err)
	}
	return nil
}
