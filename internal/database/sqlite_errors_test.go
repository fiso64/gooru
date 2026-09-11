package database

import (
	"errors"
	"fmt"
	"testing"

	sqlite "gosqlite.org"
)

func TestIsTransientSQLiteContention(t *testing.T) {
	for _, err := range []error{
		sqlite.ErrBusy,
		sqlite.ErrLocked,
		fmt.Errorf("wrapped busy: %w", sqlite.ErrBusy),
	} {
		if !IsTransientSQLiteContention(err) {
			t.Fatalf("expected transient contention for %v", err)
		}
	}
	if IsTransientSQLiteContention(errors.New("disk I/O failure")) {
		t.Fatal("non-SQLite error classified as transient contention")
	}
}
