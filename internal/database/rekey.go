package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// MigrateEncryptedDatabaseKey atomically rewrites an existing encrypted
// database from oldKey to newKey. It is intended for format/key-domain
// migrations, not as a general administrator-facing key-rotation workflow.
func MigrateEncryptedDatabaseKey(path string, oldKey, newKey []byte) error {
	if len(oldKey) != encryptedDatabaseKeySize || len(newKey) != encryptedDatabaseKeySize {
		return ErrInvalidDatabaseEncryptionKey
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	newPath, err := reserveSiblingPath(dir, "."+base+".rekeyed-")
	if err != nil {
		return fmt.Errorf("reserve rekey target path: %w", err)
	}
	defer cleanupSQLiteFiles(newPath)

	backupPath, err := reserveSiblingPath(dir, "."+base+".legacy-key-backup-")
	if err != nil {
		return fmt.Errorf("reserve rekey backup path: %w", err)
	}
	// Do not defer cleanup of backupPath. Once the source is staged there it is
	// the rollback copy, and it must survive if restoring the original path fails.

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
	if err := os.Rename(newPath, path); err != nil {
		return rollbackDatabaseMigration(fmt.Errorf("install rekeyed database: %w", err), path, backupPath)
	}

	verified, err := NewEncryptedStore(path, false, newKey)
	if err != nil {
		return rollbackDatabaseMigration(fmt.Errorf("reopen rekeyed database: %w", err), path, backupPath)
	}
	verifyErr := checkDatabaseIntegrity(verified.DB)
	closeErr := verified.Close()
	if verifyErr != nil {
		return rollbackDatabaseMigration(fmt.Errorf("verify installed rekeyed database: %w", verifyErr), path, backupPath)
	}
	if closeErr != nil {
		return rollbackDatabaseMigration(fmt.Errorf("close installed rekeyed database: %w", closeErr), path, backupPath)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove legacy-key database backup: %w", err)
	}
	cleanupSQLiteSidecars(backupPath)
	return SecureDBFiles(path)
}
