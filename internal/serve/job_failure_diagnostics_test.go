package serve

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJobFailureLoggingAddsSanitizedDiagnostics(t *testing.T) {
	var output lockedLogBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	manager := NewJobManagerWithLimits(8, 1, 0, time.Hour)

	generic, err := manager.Submit(context.Background(), "import", true, func(context.Context) (interface{}, error) {
		return nil, errors.New("private-filename.jpg failed analysis")
	})
	if err != nil {
		t.Fatalf("submit generic failure: %v", err)
	}
	waitForLoggedJobStatus(t, manager, generic.ID, JobFailed)
	waitForLogContains(t, &output, "job_type=import status=failed")
	waitForLogContains(t, &output, "error_class=internal_error")
	waitForLogContains(t, &output, "error_types=*errors.errorString")

	busyErr := sqliteBusyError(t)
	sqliteFailure, err := manager.Submit(context.Background(), "upload_import", true, func(context.Context) (interface{}, error) {
		return nil, busyErr
	})
	if err != nil {
		t.Fatalf("submit sqlite failure: %v", err)
	}
	waitForLoggedJobStatus(t, manager, sqliteFailure.ID, JobFailed)
	waitForLogContains(t, &output, "job_type=upload_import status=failed")
	waitForLogContains(t, &output, "error_class=sqlite_busy")
	waitForLogContains(t, &output, "sqlite_code=5")

	logs := output.String()
	for _, private := range []string{"private-filename.jpg", "failed analysis"} {
		if strings.Contains(logs, private) {
			t.Fatalf("job failure diagnostics leaked %q: %s", private, logs)
		}
	}
}

func sqliteBusyError(t *testing.T) error {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "busy.db")
	dsn := dbPath + "?_journal=WAL&_busy_timeout=0"
	writer, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite writer: %v", err)
	}
	defer writer.Close()
	contender, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite contender: %v", err)
	}
	defer contender.Close()
	writer.SetMaxOpenConns(1)
	contender.SetMaxOpenConns(1)

	if _, err := writer.Exec(`CREATE TABLE contention (value TEXT)`); err != nil {
		t.Fatalf("create contention table: %v", err)
	}
	tx, err := writer.Begin()
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO contention(value) VALUES ('held')`); err != nil {
		t.Fatalf("acquire writer lock: %v", err)
	}

	_, busyErr := contender.Exec(`INSERT INTO contention(value) VALUES ('blocked')`)
	if busyErr == nil {
		t.Fatal("expected SQLITE_BUSY from concurrent writer")
	}
	return busyErr
}
