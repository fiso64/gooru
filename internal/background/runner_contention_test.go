package background

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gooru.local/internal/database"
	sqlite "gosqlite.org"
)

func TestRunnerRetriesTransientSQLiteClaimContention(t *testing.T) {
	store := &fakeTaskStore{claimErr: sqlite.ErrBusy}
	runner := newTestRunner(t, store, map[string]Handler{
		"thumbnail": func(context.Context, database.BackgroundTask) error { return nil },
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := runner.Run(ctx); err != nil {
		t.Fatalf("transient SQLite contention stopped runner: %v", err)
	}
}

func TestRunnerStillFailsNonTransientClaimErrors(t *testing.T) {
	store := &fakeTaskStore{claimErr: errors.New("database corrupt")}
	runner := newTestRunner(t, store, map[string]Handler{
		"thumbnail": func(context.Context, database.BackgroundTask) error { return nil },
	})
	err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "database corrupt") {
		t.Fatalf("claim error = %v, want non-transient failure", err)
	}
}
