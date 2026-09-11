package database

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"testing"
	"time"
)

func BenchmarkBackgroundOperationLifecycleFanout(b *testing.B) {
	const fanout = 1000
	for iteration := 0; iteration < b.N; iteration++ {
		b.StopTimer()
		db, err := sql.Open("sqlite3", ":memory:")
		if err != nil {
			b.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
			_ = db.Close()
			b.Fatal(err)
		}
		if err := RunMigrations(db); err != nil {
			_ = db.Close()
			b.Fatalf("RunMigrations: %v", err)
		}
		store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}
		now := time.Date(2026, time.September, 11, 13, 0, 0, 0, time.UTC)
		operationID := fmt.Sprintf("bench-operation-%d", iteration)
		if _, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{
			ID:            operationID,
			Kind:          "files.delete",
			Visible:       true,
			ProgressTotal: fanout,
			CreatedAt:     now,
		}); err != nil {
			_ = db.Close()
			b.Fatalf("CreateBackgroundOperation: %v", err)
		}
		for taskIndex := 0; taskIndex < fanout; taskIndex++ {
			if _, created, err := store.EnqueueBackgroundTask(db, NewBackgroundTask{
				ID:            fmt.Sprintf("bench-task-%d-%d", iteration, taskIndex),
				OperationID:   operationID,
				DedupeKey:     fmt.Sprintf("bench-delete-%d-%d", iteration, taskIndex),
				Kind:          "files.delete",
				ResourceClass: "storage",
				AvailableAt:   now,
				CreatedAt:     now,
			}); err != nil || !created {
				_ = db.Close()
				b.Fatalf("EnqueueBackgroundTask(%d) = created %v, err %v", taskIndex, created, err)
			}
		}

		b.StartTimer()
		for taskIndex := 0; taskIndex < fanout; taskIndex++ {
			task, ok, err := store.ClaimNextBackgroundTask("storage", "bench-worker", now, time.Hour)
			if err != nil || !ok {
				b.Fatalf("ClaimNextBackgroundTask(%d) = ok %v, err %v", taskIndex, ok, err)
			}
			if err := store.CompleteBackgroundTask(task.ID, "bench-worker", now.Add(time.Second)); err != nil {
				b.Fatalf("CompleteBackgroundTask(%d): %v", taskIndex, err)
			}
		}
		b.StopTimer()

		var status string
		var completed int64
		if err := db.QueryRow(`SELECT status, progress_completed FROM background_operations WHERE id = ?`, operationID).Scan(&status, &completed); err != nil {
			_ = db.Close()
			b.Fatal(err)
		}
		if status != string(BackgroundWorkCompleted) || completed != fanout {
			_ = db.Close()
			b.Fatalf("operation status/completed = %s/%d, want completed/%d", status, completed, fanout)
		}
		if err := db.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
