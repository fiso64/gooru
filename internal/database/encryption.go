package database

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	sqlite "gosqlite.org"
	"gosqlite.org/vfs/crypto"
)

const (
	encryptedDatabaseKeySize          = 32
	plaintextEncryptionTargetSuffix = ".gooru-enable-encrypted"
	plaintextEncryptionBackupSuffix = ".gooru-enable-plaintext-backup"
)

var (
	ErrInvalidDatabaseEncryptionKey = errors.New("database encryption key must be exactly 32 bytes")
	ErrDatabaseNotPlaintext         = errors.New("database is not a plaintext SQLite database")
)

var sqliteFileHeader = []byte("SQLite format 3\x00")

// NewEncryptedStore opens a SQLite database through gosqlite's page-encryption
// VFS. The VFS encrypts the main database, journal/WAL pages, and temporary
// files. Store.Close owns both the database pool and the registered VFS.
func NewEncryptedStore(dataSourceName string, verbose bool, key []byte) (*Store, error) {
	if len(key) != encryptedDatabaseKeySize {
		return nil, ErrInvalidDatabaseEncryptionKey
	}

	db, err := crypto.Open(
		sqlite.Config{Path: dataSourceName, Pragmas: sqlite.RecommendedPragmas(), TxLock: "immediate"},
		crypto.Options{Key: key},
	)
	if err != nil {
		return nil, fmt.Errorf("open encrypted database: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping encrypted database: %w", err)
	}
	if err := SecureDBFiles(dataSourceName); err != nil {
		_ = db.Close()
		return nil, err
	}

	var logOutput io.Writer = io.Discard
	if verbose {
		logOutput = os.Stderr
	}
	logger := log.New(logOutput, "SQL: ", log.Ltime|log.Lmicroseconds)

	return &Store{
		DB:             db.DB,
		dataSourceName: dataSourceName,
		logger:         logger,
		backendClose:   db.Close,
	}, nil
}

// IsPlaintextDatabase reports whether path begins with SQLite's cleartext file
// header. Encrypted databases intentionally do not expose that header.
func IsPlaintextDatabase(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	header := make([]byte, len(sqliteFileHeader))
	if _, err := io.ReadFull(file, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, fmt.Errorf("read database header: %w", err)
	}
	return bytes.Equal(header, sqliteFileHeader), nil
}

// RecoverPlaintextDatabaseEncryptionMigration repairs the deterministic swap
// state left by an interrupted plaintext-to-encrypted migration. A staged
// plaintext source is restored without needing the encryption key. If the
// encrypted database is already canonical, key is required to verify it before
// the plaintext rollback copy is removed; an empty key leaves that copy in
// place for a later keyed recovery pass.
func RecoverPlaintextDatabaseEncryptionMigration(path string, key []byte) error {
	if len(key) != 0 && len(key) != encryptedDatabaseKeySize {
		return ErrInvalidDatabaseEncryptionKey
	}
	targetPath := path + plaintextEncryptionTargetSuffix
	backupPath := path + plaintextEncryptionBackupSuffix

	backupPresent, err := recoverInterruptedDatabaseSwap(path, targetPath, backupPath)
	if err != nil {
		return fmt.Errorf("recover plaintext database encryption migration: %w", err)
	}
	if !backupPresent || len(key) == 0 {
		return nil
	}

	plain, err := IsPlaintextDatabase(path)
	if err != nil {
		return fmt.Errorf("inspect installed database during encryption recovery: %w", err)
	}
	if plain {
		return fmt.Errorf("ambiguous plaintext database encryption recovery state: canonical database is plaintext while rollback backup remains at %q", backupPath)
	}

	return finalizeRecoveredDatabaseMigration(path, backupPath, "encrypted database", func() (*Store, error) {
		return NewEncryptedStore(path, false, key)
	})
}

// MigratePlaintextDatabase rewrites an existing plaintext SQLite database into
// an encrypted database at the same path. The copy is built and integrity-
// checked beside the source, then swapped into place only after both databases
// are closed. The original is retained just long enough to make the rename
// reversible and is removed after the encrypted replacement reopens cleanly.
//
// The caller must ensure no other process is using the database during this
// one-time migration.
func MigratePlaintextDatabase(path string, key []byte) error {
	if len(key) != encryptedDatabaseKeySize {
		return ErrInvalidDatabaseEncryptionKey
	}
	if err := RecoverPlaintextDatabaseEncryptionMigration(path, key); err != nil {
		return err
	}
	plain, err := IsPlaintextDatabase(path)
	if err != nil {
		return err
	}
	if !plain {
		return ErrDatabaseNotPlaintext
	}

	dir := filepath.Dir(path)
	encryptedPath := path + plaintextEncryptionTargetSuffix
	backupPath := path + plaintextEncryptionBackupSuffix
	cleanupSQLiteFiles(encryptedPath)
	defer cleanupSQLiteFiles(encryptedPath)

	source, err := NewStore(path, false)
	if err != nil {
		return fmt.Errorf("open plaintext database for migration: %w", err)
	}
	sourceOpen := true
	defer func() {
		if sourceOpen {
			_ = source.Close()
		}
	}()

	// Make the source main file a complete rollback copy before sidecars are
	// discarded during the final swap.
	if _, err := source.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint plaintext database: %w", err)
	}

	destination, err := NewEncryptedStore(encryptedPath, false, key)
	if err != nil {
		return fmt.Errorf("create encrypted migration target: %w", err)
	}
	destinationOpen := true
	defer func() {
		if destinationOpen {
			_ = destination.Close()
		}
	}()

	if err := backupDatabase(context.Background(), source.DB, destination.DB); err != nil {
		return fmt.Errorf("copy database into encrypted target: %w", err)
	}
	if _, err := destination.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint encrypted migration target: %w", err)
	}
	if err := checkDatabaseIntegrity(destination.DB); err != nil {
		return fmt.Errorf("verify encrypted migration target: %w", err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("close encrypted migration target: %w", err)
	}
	destinationOpen = false
	if err := source.Close(); err != nil {
		return fmt.Errorf("close plaintext migration source: %w", err)
	}
	sourceOpen = false

	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("stage plaintext database for replacement: %w", err)
	}
	cleanupSQLiteSidecars(path)
	if err := syncDatabaseDir(dir); err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("sync staged plaintext database replacement: %w", err), path, backupPath)
	}

	if err := os.Rename(encryptedPath, path); err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("install encrypted database: %w", err), path, backupPath)
	}
	if err := syncDatabaseDir(dir); err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("sync installed encrypted database: %w", err), path, backupPath)
	}

	verified, err := NewEncryptedStore(path, false, key)
	if err != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("reopen encrypted database after migration: %w", err), path, backupPath)
	}
	verifyErr := checkDatabaseIntegrity(verified.DB)
	closeErr := verified.Close()
	if verifyErr != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("verify installed encrypted database: %w", verifyErr), path, backupPath)
	}
	if closeErr != nil {
		return rollbackDatabaseMigrationDurably(fmt.Errorf("close installed encrypted database: %w", closeErr), path, backupPath)
	}

	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("remove plaintext migration backup: %w", err)
	}
	cleanupSQLiteSidecars(backupPath)
	if err := syncDatabaseDir(dir); err != nil {
		return fmt.Errorf("sync completed encrypted database migration: %w", err)
	}
	return SecureDBFiles(path)
}

func backupDatabase(ctx context.Context, source, destination *sql.DB) error {
	sourceConn, err := source.Conn(ctx)
	if err != nil {
		return err
	}
	defer sourceConn.Close()
	destinationConn, err := destination.Conn(ctx)
	if err != nil {
		return err
	}
	defer destinationConn.Close()

	return sourceConn.Raw(func(sourceDriver any) error {
		sourceSQLite, ok := sourceDriver.(*sqlite.Conn)
		if !ok {
			return fmt.Errorf("unexpected source sqlite driver %T", sourceDriver)
		}
		return destinationConn.Raw(func(destinationDriver any) error {
			destinationSQLite, ok := destinationDriver.(*sqlite.Conn)
			if !ok {
				return fmt.Errorf("unexpected destination sqlite driver %T", destinationDriver)
			}
			backup, err := destinationSQLite.Backup("main", sourceSQLite, "main")
			if err != nil {
				return err
			}
			defer backup.Close()
			more, err := backup.Step(-1)
			if err != nil {
				return err
			}
			if more {
				return errors.New("sqlite backup did not finish in a full-copy step")
			}
			return backup.Finish()
		})
	})
}

func checkDatabaseIntegrity(db *sql.DB) error {
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check returned %q", result)
	}
	return nil
}

func recoverInterruptedDatabaseSwap(path, targetPath, backupPath string) (bool, error) {
	_, pathErr := os.Stat(path)
	_, backupErr := os.Stat(backupPath)
	switch {
	case os.IsNotExist(pathErr) && backupErr == nil:
		if err := os.Rename(backupPath, path); err != nil {
			return false, fmt.Errorf("restore staged database from %q: %w", backupPath, err)
		}
		cleanupSQLiteFiles(targetPath)
		if err := syncDatabaseDir(filepath.Dir(path)); err != nil {
			return false, fmt.Errorf("sync restored database directory: %w", err)
		}
		return false, nil
	case pathErr != nil:
		if os.IsNotExist(pathErr) && os.IsNotExist(backupErr) {
			cleanupSQLiteFiles(targetPath)
			return false, nil
		}
		return false, fmt.Errorf("inspect canonical database during migration recovery: %w", pathErr)
	case backupErr != nil && !os.IsNotExist(backupErr):
		return false, fmt.Errorf("inspect database rollback backup during migration recovery: %w", backupErr)
	}

	cleanupSQLiteFiles(targetPath)
	return backupErr == nil, nil
}

func finalizeRecoveredDatabaseMigration(path, backupPath, description string, open func() (*Store, error)) error {
	store, err := open()
	if err != nil {
		return fmt.Errorf("reopen %s during migration recovery: %w", description, err)
	}
	verifyErr := checkDatabaseIntegrity(store.DB)
	closeErr := store.Close()
	if verifyErr != nil {
		return fmt.Errorf("verify %s during migration recovery: %w", description, verifyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s during migration recovery: %w", description, closeErr)
	}
	if err := os.Remove(backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove completed database migration backup %q: %w", backupPath, err)
	}
	cleanupSQLiteSidecars(backupPath)
	if err := syncDatabaseDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync completed database migration recovery: %w", err)
	}
	return SecureDBFiles(path)
}

func rollbackDatabaseMigration(migrationErr error, path, backupPath string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w; rollback could not remove failed replacement: %v; original database remains at %q", migrationErr, err, backupPath)
	}
	cleanupSQLiteSidecars(path)
	if err := os.Rename(backupPath, path); err != nil {
		return fmt.Errorf("%w; rollback could not restore original database: %v; original database remains at %q", migrationErr, err, backupPath)
	}
	return migrationErr
}

func rollbackDatabaseMigrationDurably(migrationErr error, path, backupPath string) error {
	rollbackErr := rollbackDatabaseMigration(migrationErr, path, backupPath)
	if err := syncDatabaseDir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("%w; sync database rollback directory: %v", rollbackErr, err)
	}
	return rollbackErr
}

func cleanupSQLiteFiles(path string) {
	_ = os.Remove(path)
	cleanupSQLiteSidecars(path)
}

func cleanupSQLiteSidecars(path string) {
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		_ = os.Remove(path + suffix)
	}
}
