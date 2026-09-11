package database

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestForegroundTransactionReadThenWriteSurvivesConcurrentCommit(t *testing.T) {
	tests := []struct {
		name string
		open func(t *testing.T, path string) *Store
	}{
		{
			name: "plaintext",
			open: func(t *testing.T, path string) *Store {
				t.Helper()
				store, err := NewStore(path, false)
				if err != nil {
					t.Fatalf("NewStore: %v", err)
				}
				return store
			},
		},
		{
			name: "encrypted",
			open: func(t *testing.T, path string) *Store {
				t.Helper()
				store, err := NewEncryptedStore(path, false, bytes.Repeat([]byte{0x42}, encryptedDatabaseKeySize))
				if err != nil {
					t.Fatalf("NewEncryptedStore: %v", err)
				}
				return store
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.open(t, filepath.Join(t.TempDir(), "gooru.db"))
			defer store.Close()
			store.DB.SetMaxOpenConns(4)

			if _, err := store.Exec(`CREATE TABLE tx_upgrade (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
				t.Fatalf("create table: %v", err)
			}
			if _, err := store.Exec(`INSERT INTO tx_upgrade(id, value) VALUES (1, 'initial')`); err != nil {
				t.Fatalf("seed row: %v", err)
			}

			tx, err := store.Begin()
			if err != nil {
				t.Fatalf("begin foreground transaction: %v", err)
			}
			defer tx.Rollback()

			var value string
			if err := tx.QueryRow(`SELECT value FROM tx_upgrade WHERE id = 1`).Scan(&value); err != nil {
				t.Fatalf("establish read snapshot: %v", err)
			}
			if value != "initial" {
				t.Fatalf("initial value = %q, want initial", value)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			concurrentDone := make(chan error, 1)
			go func() {
				_, err := store.DB.ExecContext(ctx, `UPDATE tx_upgrade SET value = 'concurrent' WHERE id = 1`)
				concurrentDone <- err
			}()

			// A write-intent transaction owns the writer reservation already, so
			// the competing writer waits here instead of invalidating our snapshot.
			time.Sleep(50 * time.Millisecond)
			if _, err := tx.Exec(`UPDATE tx_upgrade SET value = 'foreground' WHERE id = 1`); err != nil {
				t.Fatalf("read-then-write foreground transaction failed under concurrent writer: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit foreground transaction: %v", err)
			}

			select {
			case err := <-concurrentDone:
				if err != nil {
					t.Fatalf("queued concurrent writer failed: %v", err)
				}
			case <-ctx.Done():
				t.Fatalf("queued concurrent writer did not complete: %v", ctx.Err())
			}
		})
	}
}
