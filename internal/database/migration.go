package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed migrations
var migrationsFS embed.FS

const (
	migrationTable                              = "schema_migrations"
	historicalMediaMetadataCollisionVersion    = 38
	historicalMediaMetadataCollisionMaxVersion = 39
	historicalMediaMetadataMigrationName       = "038_media_metadata_content_identity.up.sql"
)

type embeddedMigration struct {
	version int
	name    string
	sql     string
}

var (
	embeddedMigrationsOnce sync.Once
	embeddedMigrations     []embeddedMigration
	embeddedMigrationsErr  error
)

// RunMigrations applies embedded up migrations to the exact database connection
// supplied by the caller. The runner deliberately depends only on database/sql,
// so connection-specific storage backends (including encrypted SQLite VFSes) can
// be introduced without importing a migration driver that registers its own
// competing SQLite database/sql driver.
//
// The schema_migrations table remains compatible with the golang-migrate SQLite
// layout used by older Gooru releases: one (version, dirty) row and a unique
// version index. A failed migration is left dirty so startup fails closed rather
// than attempting later migrations on an uncertain schema.
func RunMigrations(db *sql.DB) error {
	migrations, err := loadEmbeddedMigrations()
	if err != nil {
		return err
	}
	if err := ensureMigrationTable(db); err != nil {
		return err
	}

	current, dirty, err := currentMigrationVersion(db)
	if err != nil {
		return err
	}
	if dirty {
		return fmt.Errorf("database migration version %d is dirty", current)
	}
	if err := repairHistoricalMediaMetadataMigrationCollision(db, migrations, current); err != nil {
		return err
	}

	for _, migration := range migrations {
		if migration.version <= current {
			continue
		}
		if err := applyMigration(db, migration); err != nil {
			return err
		}
		current = migration.version
	}
	return nil
}

// repairHistoricalMediaMetadataMigrationCollision repairs databases created by
// the long-lived #670 development branch before it incorporated the content-
// identity metadata migration. That branch briefly used migration version 38
// for a different trigger-only change. Because schema_migrations records only a
// numeric version, a database that ran that historical migration later skipped
// the canonical 038_media_metadata_content_identity migration and could advance
// to version 39 while still retaining media_metadata.location_id.
//
// Only versions that the collided development branch could have recorded are
// eligible. The canonical migration body is replayed without changing the
// ledger, preserving an already-applied version 39. Valid content-identity
// schemas are a no-op and unexpected schemas fail closed.
func repairHistoricalMediaMetadataMigrationCollision(db *sql.DB, migrations []embeddedMigration, current int) error {
	if current < historicalMediaMetadataCollisionVersion || current > historicalMediaMetadataCollisionMaxVersion {
		return nil
	}

	columns, err := tableColumnNames(db, "media_metadata")
	if err != nil {
		return fmt.Errorf("inspect media_metadata for historical migration collision: %w", err)
	}
	if columns["content_hash"] {
		return nil
	}
	if !columns["location_id"] {
		return fmt.Errorf("media_metadata schema at migration version %d has neither content_hash nor legacy location_id", current)
	}

	var repair embeddedMigration
	found := false
	for _, migration := range migrations {
		if migration.version == historicalMediaMetadataCollisionVersion && migration.name == historicalMediaMetadataMigrationName {
			repair = migration
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("historical media metadata migration repair %q is unavailable", historicalMediaMetadataMigrationName)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin historical media metadata migration repair: %w", err)
	}
	if _, err := tx.Exec(repair.sql); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("repair historical media metadata migration collision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit historical media metadata migration repair: %w", err)
	}
	return nil
}

func tableColumnNames(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func loadEmbeddedMigrations() ([]embeddedMigration, error) {
	embeddedMigrationsOnce.Do(func() {
		embeddedMigrations, embeddedMigrationsErr = readEmbeddedMigrations()
	})
	return embeddedMigrations, embeddedMigrationsErr
}

func readEmbeddedMigrations() ([]embeddedMigration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	migrations := make([]embeddedMigration, 0, len(entries)/2)
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if previous, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, entry.Name())
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		seen[version] = entry.Name()
		migrations = append(migrations, embeddedMigration{version: version, name: entry.Name(), sql: string(body)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

func ensureMigrationTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS schema_migrations (version uint64, dirty bool);
CREATE UNIQUE INDEX IF NOT EXISTS version_unique ON schema_migrations (version);
`)
	if err != nil {
		return fmt.Errorf("ensure migration table: %w", err)
	}
	return nil
}

func currentMigrationVersion(db *sql.DB) (version int, dirty bool, err error) {
	err = db.QueryRow(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&version, &dirty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("read migration version: %w", err)
	}
	return version, dirty, nil
}

func setMigrationVersion(db *sql.DB, version int, dirty bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration version update: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM schema_migrations`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clear migration version: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)`, version, dirty); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("write migration version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration version update: %w", err)
	}
	return nil
}

func applyMigration(db *sql.DB, migration embeddedMigration) error {
	if err := setMigrationVersion(db, migration.version, true); err != nil {
		return fmt.Errorf("mark migration %s dirty: %w", migration.name, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", migration.name, err)
	}
	if _, err := tx.Exec(migration.sql); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %s: %w", migration.name, err)
	}
	if _, err := tx.Exec(`DELETE FROM schema_migrations`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clear migration version after %s: %w", migration.name, err)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations (version, dirty) VALUES (?, ?)`, migration.version, false); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("mark migration %s clean: %w", migration.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", migration.name, err)
	}
	return nil
}
