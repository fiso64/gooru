package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

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
	plain, err := IsPlaintextDatabase(path)
	if err != nil {
		return err
	}
	if plain {
		return nil
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	plaintextPath, err := reserveSiblingPath(dir, "."+base+".plaintext-")
	if err != nil {
		return fmt.Errorf("reserve plaintext database path: %w", err)
	}
	defer cleanupSQLiteFiles(plaintextPath)

	backupPath, err := reserveSiblingPath(dir, "."+base+".encrypted-backup-")
	if err != nil {
		return fmt.Errorf("reserve encrypted database backup path: %w", err)
	}
	defer cleanupSQLiteFiles(backupPath)

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
	if err := os.Rename(plaintextPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("install plaintext database: %w", err)
	}

	verified, err := NewStore(path, false)
	if err != nil {
		_ = os.Remove(path)
		_ = os.Rename(backupPath, path)
		return fmt.Errorf("reopen plaintext database after migration: %w", err)
	}
	verifyErr := checkDatabaseIntegrity(verified.DB)
	closeErr := verified.Close()
	if verifyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		_ = os.Rename(backupPath, path)
		if verifyErr != nil {
			return fmt.Errorf("verify installed plaintext database: %w", verifyErr)
		}
		return fmt.Errorf("close installed plaintext database: %w", closeErr)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove encrypted migration backup: %w", err)
	}
	cleanupSQLiteSidecars(backupPath)
	return SecureDBFiles(path)
}
