package database

import (
	"context"
	"testing"
	"time"
)

func TestNewStoreWaitsForConcurrentWriter(t *testing.T) {
	store, err := NewStore(t.TempDir()+"/gooru.db", false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	store.DB.SetMaxOpenConns(4)

	if _, err := store.Exec(`CREATE TABLE writer_contention (id INTEGER PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	tx, err := store.DB.Begin()
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO writer_contention(value) VALUES ('first')`); err != nil {
		t.Fatalf("acquire writer lock: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := store.DB.ExecContext(ctx, `INSERT INTO writer_contention(value) VALUES ('second')`)
		result <- err
	}()

	// Keep the first transaction alive long enough that a fail-fast connection
	// reliably observes the lock. With busy_timeout configured on every pooled
	// connection, the second writer waits here and succeeds after the commit.
	time.Sleep(100 * time.Millisecond)
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit first writer: %v", err)
	}

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("concurrent writer failed instead of waiting: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("concurrent writer did not complete: %v", ctx.Err())
	}

	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM writer_contention`).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("row count = %d, want 2", count)
	}
}
