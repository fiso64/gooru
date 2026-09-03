package database

import (
	"context"
	"database/sql/driver"

	sqlite "gosqlite.org"
)

const sqliteBusyTimeoutMilliseconds = 5000

func init() {
	// NewStore historically configures WAL and foreign keys through its DSN, but
	// did not configure a busy timeout. SQLite's default timeout is zero, so a
	// second pooled connection trying to write while another transaction owns
	// the writer lock fails immediately with SQLITE_BUSY. Apply the timeout as a
	// connection hook so it covers every connection opened by the shared driver,
	// including connections added to an existing database/sql pool later.
	sqlite.DefaultDriver().RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, _ string) error {
		_, err := conn.ExecContext(
			context.Background(),
			"PRAGMA busy_timeout = 5000",
			[]driver.NamedValue{},
		)
		return err
	})
}
