package database

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackDatabaseMigrationRestoresBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	backupPath := filepath.Join(dir, "library.db.backup")
	if err := os.WriteFile(path, []byte("failed replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("original database"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrationErr := errors.New("migration failed")
	if err := rollbackDatabaseMigration(migrationErr, path, backupPath); !errors.Is(err, migrationErr) {
		t.Fatalf("rollback error = %v, want migration error", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original database" {
		t.Fatalf("restored database = %q, want original database", got)
	}
	if _, err := os.Stat(backupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup path still exists after successful restore: %v", err)
	}
}

func TestRollbackDatabaseMigrationPreservesBackupWhenRestoreBlocked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "library.db")
	backupPath := filepath.Join(dir, "library.db.backup")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "block-removal"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupPath, []byte("original database"), 0o600); err != nil {
		t.Fatal(err)
	}

	migrationErr := errors.New("migration failed")
	err := rollbackDatabaseMigration(migrationErr, path, backupPath)
	if !errors.Is(err, migrationErr) {
		t.Fatalf("rollback error = %v, want migration error", err)
	}
	if !strings.Contains(err.Error(), "original database remains at") {
		t.Fatalf("rollback error = %q, want preserved-backup location", err)
	}

	got, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		t.Fatalf("read preserved backup: %v", readErr)
	}
	if string(got) != "original database" {
		t.Fatalf("preserved backup = %q, want original database", got)
	}
}
