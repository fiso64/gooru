package database

import (
	"database/sql"
	"testing"
)

func TestDurableBackgroundWorkMigrationSupportsHistoryAndOwnership(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO background_operations (id, kind, progress_total, created_at)
		VALUES ('op-1', 'upload', 2, 100)
	`); err != nil {
		t.Fatalf("insert operation: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO background_tasks
			(id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key, resource_class, priority, available_at, created_at)
		VALUES
			('task-1', 'op-1', 'thumbnail:file-1', 'thumbnail', 'file', 'file-1', 'processor-v1', 'image', 10, 100, 100)
	`); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO background_tasks (id, dedupe_key, kind, available_at, created_at)
		VALUES ('task-duplicate', 'thumbnail:file-1', 'thumbnail', 100, 100)
	`); err == nil {
		t.Fatal("active duplicate dedupe key unexpectedly succeeded")
	}

	if _, err := db.Exec(`
		INSERT INTO background_task_attempts (task_id, attempt_number, worker_id, started_at)
		VALUES ('task-1', 1, 'worker-a', 101)
	`); err != nil {
		t.Fatalf("insert task attempt: %v", err)
	}

	if _, err := db.Exec(`UPDATE background_tasks SET status = 'completed', finished_at = 110 WHERE id = 'task-1'`); err != nil {
		t.Fatalf("complete task: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO background_tasks
			(id, operation_id, dedupe_key, kind, subject_kind, subject_id, input_key, available_at, created_at)
		VALUES
			('task-2', 'op-1', 'thumbnail:file-1', 'thumbnail', 'file', 'file-1', 'processor-v2', 120, 120)
	`); err != nil {
		t.Fatalf("reuse terminal dedupe key for later processing: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM background_operations WHERE id = 'op-1'`); err != nil {
		t.Fatalf("delete operation: %v", err)
	}
	var operationID sql.NullString
	if err := db.QueryRow(`SELECT operation_id FROM background_tasks WHERE id = 'task-1'`).Scan(&operationID); err != nil {
		t.Fatalf("read detached task: %v", err)
	}
	if operationID.Valid {
		t.Fatalf("deleted operation left task parent %q, want NULL", operationID.String)
	}

	if _, err := db.Exec(`DELETE FROM background_tasks WHERE id = 'task-1'`); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	var attempts int
	if err := db.QueryRow(`SELECT count(*) FROM background_task_attempts WHERE task_id = 'task-1'`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("deleted task left %d attempt rows, want 0", attempts)
	}
}

func TestAttachedProgressTotalMigrationRoundTrips(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	down, err := migrationsFS.ReadFile("migrations/034_background_operation_attached_progress_total.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(down)); err != nil {
		t.Fatalf("apply migration 034 down: %v", err)
	}

	var attachedColumnCount int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('background_operations') WHERE name = 'attached_task_count'`).Scan(&attachedColumnCount); err != nil {
		t.Fatal(err)
	}
	if attachedColumnCount != 0 {
		t.Fatalf("attached_task_count columns after down migration = %d, want 0", attachedColumnCount)
	}

	if _, err := db.Exec(`
		INSERT INTO background_operations (id, kind, status, progress_total, progress_completed, created_at, finished_at)
		VALUES ('roundtrip-op', 'dynamic', 'completed', 1, 1, 100, 110)
	`); err != nil {
		t.Fatalf("insert completed operation after down migration: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO background_tasks (id, operation_id, dedupe_key, kind, available_at, created_at)
		VALUES ('roundtrip-task-1', 'roundtrip-op', 'roundtrip-task-1', 'dynamic', 120, 120)
	`); err != nil {
		t.Fatalf("attach task using restored migration 033 trigger: %v", err)
	}
	var status BackgroundWorkStatus
	var total int64
	if err := db.QueryRow(`SELECT status, progress_total FROM background_operations WHERE id = 'roundtrip-op'`).Scan(&status, &total); err != nil {
		t.Fatal(err)
	}
	if status != BackgroundWorkPending || total != 1 {
		t.Fatalf("migration 033 behavior after down = status %q total %d, want pending/1", status, total)
	}

	up, err := migrationsFS.ReadFile("migrations/034_background_operation_attached_progress_total.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(up)); err != nil {
		t.Fatalf("reapply migration 034 up: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO background_tasks (id, operation_id, dedupe_key, kind, available_at, created_at)
		VALUES ('roundtrip-task-2', 'roundtrip-op', 'roundtrip-task-2', 'dynamic', 130, 130)
	`); err != nil {
		t.Fatalf("attach task after reapplying migration 034: %v", err)
	}
	if err := db.QueryRow(`SELECT progress_total FROM background_operations WHERE id = 'roundtrip-op'`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("progress_total after reapplying migration 034 and attaching child = %d, want 2", total)
	}
}
