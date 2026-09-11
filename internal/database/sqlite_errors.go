package database

import (
	"errors"

	sqlite "gosqlite.org"
)

// IsTransientSQLiteContention reports whether err represents SQLite lock
// contention that can be retried without changing durable state. Keep this
// classification at the database boundary so background scheduling does not
// depend on driver error strings.
func IsTransientSQLiteContention(err error) bool {
	return errors.Is(err, sqlite.ErrBusy) || errors.Is(err, sqlite.ErrLocked)
}
