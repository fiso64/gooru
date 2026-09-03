package serve

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
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

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE diagnostic_constraint (value TEXT UNIQUE)`); err != nil {
		t.Fatalf("create constraint table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO diagnostic_constraint(value) VALUES ('duplicate')`); err != nil {
		t.Fatalf("seed constraint table: %v", err)
	}
	_, constraintErr := db.Exec(`INSERT INTO diagnostic_constraint(value) VALUES ('duplicate')`)
	if constraintErr == nil {
		t.Fatal("expected sqlite constraint error")
	}

	sqliteFailure, err := manager.Submit(context.Background(), "upload_import", true, func(context.Context) (interface{}, error) {
		return nil, constraintErr
	})
	if err != nil {
		t.Fatalf("submit sqlite failure: %v", err)
	}
	waitForLoggedJobStatus(t, manager, sqliteFailure.ID, JobFailed)
	waitForLogContains(t, &output, "job_type=upload_import status=failed")
	waitForLogContains(t, &output, "error_class=sqlite_constraint")
	waitForLogContains(t, &output, "sqlite_code=19")

	logs := output.String()
	for _, private := range []string{"private-filename.jpg", "failed analysis", "duplicate"} {
		if strings.Contains(logs, private) {
			t.Fatalf("job failure diagnostics leaked %q: %s", private, logs)
		}
	}
}
