package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations
var migrationsFS embed.FS

// RunMigrations checks the current database schema version and applies any
// pending migrations. It's safe to call on every application startup.
func RunMigrations(db *sql.DB, dbPath string) error {
	// By embedding the entire 'migrations' directory, the source driver needs to be
	// told to look for .sql files inside that directory path within the virtual FS.
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("could not create migration source driver: %w", err)
	}

	// 1. Register our existing database connection with the migrate library.
	// This allows the library to use our connection instead of creating a new one.
	_, err = sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		return fmt.Errorf("could not create migration database driver: %w", err)
	}

	// 2. Create the migrate instance. We provide our embedded .sql files as the source
	// and a properly formatted URL to the database.
	// For file paths, especially on Windows, we must use the file:///C:/... format
	// to prevent the URL parser from misinterpreting the drive letter as a host.
	urlPath := filepath.ToSlash(dbPath)
	if runtime.GOOS == "windows" {
		// For a path like C:\Users\..., ToSlash gives C:/Users/...
		// The URL needs to be /C:/Users/... to be parsed correctly.
		urlPath = "/" + urlPath
	}
	m, err := migrate.NewWithSourceInstance(
		"iofs",
		sourceDriver,
		"sqlite3://"+urlPath,
	)
	if err != nil {
		return fmt.Errorf("could not create migrate instance: %w", err)
	}
	// Defer closing the source and database drivers.
	// The returned errors can be ignored as the main migration error is more important.
	defer m.Close()

	// 3. Apply all available "up" migrations.
	err = m.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("an error occurred while running migrations: %w", err)
	}

	return nil
}