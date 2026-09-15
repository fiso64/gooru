package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const (
	encryptedRekeyTargetSuffix = ".gooru-rekeyed"
	encryptedRekeyBackupSuffix = ".gooru-legacy-key-backup"
)

// RecoverEncryptedDatabaseKeyMigration repairs the deterministic swap state
// left by an interrupted legacy-key to database-subkey migration. A staged
// legacy source is restored without needing either encryption key. If the
// rekeyed database is already canonical, newKey is required to verify it before
// the legacy-key rollback copy is removed; an empty newKey leaves that copy in
// place for a later keyed recovery pass.
func RecoverEncryptedDatabaseKeyMigration(path string, newKey []byte) error {
	if len(newKey) != 0 && len(newKey) != encryptedDatabaseKeySize {
		return ErrInvalidDatabaseEncryptionKey
	}
	targetPath := path + encryptedRekeyTargetSuffix
	backupPath := path + encryptedRekeyBackupSuffix

	backupPresent, err := recoverInterruptedDatabaseSwap(path, targetPath, backupPath)
	if err != nil {
		return fmt.Errorf("recover encrypted database key migration: %w", err)
	}
	if !backupPresent || len(newKey) == 0 {
		return nil
	}

	return finalizeRecoveredDatabaseMigration(path, backupPath, "rekeyed encrypted database", func() (*Store, error) {
		return NewEncryptedStore(path, false, newKey)
	})
}

// MigrateEncryptedDatabaseKey atomically rewrites an existing encrypted
// database from oldKey to newKey. It is intended for format/key-domain
// migrations, not as a general administrator-facing key-rotation workflow.
func MigrateEncryptedDatabaseKey(path string, oldKey, newKey []byte) error {
	if len(oldKey) != encryptedDatabaseKeySize || len(newKey) != encryptedDatabaseKeySize {
		return ErrInvalidDatabaseEncryptionKey
	}
	if err := RecoverEncryptedDatabaseKeyMigration(path, newKey); err != nil {
		return err
	}
	if current, err := NewEncryptedStore(path, false, newKey); err == nil {
		return current.Close()
	}

	dir := filepath.Dir(path)
	newPath := path + encryptedRekeyTargetSuffix
	backupPath := path + encryptedRekeyBackupSuffix
	cleanupSQLiteFiles(newPath)
	defer cleanupSQLiteFiles(newPath)

	source, err := NewEncryptedStore(path, false, oldKey)
	if err != nil {
		return fmt.Errorf("open legacy-key database: %w", err)
	}
	sourceOpen := true
	defer func() {
		if sourceOpen {
			_ = source.Close()
		}
	}()
	if _, err := source.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint legacy-key database: %w", err)
	}

	destination, err := NewEncryptedStore(newPath, false, newKey)
	if err != nil {
		return fmt.Errorf("create rekey target: %w", err)
	}
	destinationOpen := true
	defer func() {
		if destinationOpen {
			_ = destination.Close()
		}
	}()

	if err := backupDatabase(context.Background(), source.DB, destination.DB); err != nil {
		return fmt.Errorf("copy database into rekey target: %w", err)
	}
	if _, err := destination.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint rekey target: %w", err)
	}
	if err := checkDatabaseIntegrity(destination.DB); err != nil {
		return fmt.Errorf("verify rekey target: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close rekey target: %w", err)
	}
	destinationOpen = false
	if err := source.Close(); err != nil {
		return fmt.Errorf("close legacy-key database: %w", err)
	}
	sourceOpen = false

	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("stage legacy-key database for replacement: %w", err)
	}
	cleanupSQLiteSidecars(path)
	if err := syncDatabaseDir(dir); err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("sync staged legacy-key database replacement: %w", err), path, backupPath)
	}
	if err := os.Rename(newPath, path); err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("install rekeyed database: %w", err), path, backupPath)
	}
	if err := syncDatabaseDir(dir); err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("sync installed rekeyed database: %w", err), path, backupPath)
	}

	verified, err := NewEncryptedStore(path, false, newKey)
	if err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("reopen rekeyed database: %w", err), path, backupPath)
	}
	verifyErr := checkDatabaseIntegrity(verified.DB)
	closeErr := verified.Close()
	if verifyErr != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("verify installed rekeyed database: %w", verifyErr), path, backupPath)
	}
	if closeErr != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("close installed rekeyed database: %w", closeErr), path, backupPath)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove legacy-key database backup: %w", err)
	}
	cleanupSQLiteSidecars(backupPath)
	if err := syncDatabaseDir(dir); err != nil {
		return fmt.Errorf("sync completed database rekey migration: %w", err)
	}
	return SecureDBFiles(path)
}
