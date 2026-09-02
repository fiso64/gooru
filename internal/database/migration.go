package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations
var migrationsFS embed.FS

// RunMigrations checks the current database schema version and applies any
// pending migrations on the exact database connection supplied by the caller.
// This is important for connection-specific storage backends such as encrypted
// SQLite VFSes: reopening by path could bypass the configured backend entirely.
func RunMigrations(db *sql.DB) error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("could not create migration source driver: %w", err)
	}
	defer sourceDriver.Close()

	databaseDriver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("could not create migration database driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite3", databaseDriver)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}
	// Do not call m.Close(): sqlite3.WithInstance wraps the caller-owned *sql.DB,
	// and the migrate driver's Close method would close that live connection.

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("an error occurred while running migrations: %w", err)
	}
	return nil
}
