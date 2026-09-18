package database

import (
	"database/sql"
	"testing"
)

type countingExpiredAttemptQuerier struct {
	argCounts []int
}

func (q *countingExpiredAttemptQuerier) Exec(_ string, args ...interface{}) (sql.Result, error) {
	q.argCounts = append(q.argCounts, len(args))
	return expiredAttemptRowsAffected((len(args) - 1) / 2), nil
}

func (q *countingExpiredAttemptQuerier) Query(string, ...interface{}) (*sql.Rows, error) {
	panic("unexpected Query")
}

func (q *countingExpiredAttemptQuerier) QueryRow(string, ...interface{}) *sql.Row {
	panic("unexpected QueryRow")
}

type expiredAttemptRowsAffected int64

func (r expiredAttemptRowsAffected) LastInsertId() (int64, error) { return 0, nil }
func (r expiredAttemptRowsAffected) RowsAffected() (int64, error) { return int64(r), nil }

func TestAbandonExpiredBackgroundTaskAttemptsBatchesWithinVariableBudget(t *testing.T) {
	tasks := make([]expiredBackgroundTask, 900)
	for i := range tasks {
		tasks[i] = expiredBackgroundTask{id: "task", attemptNumber: 1}
	}
	q := &countingExpiredAttemptQuerier{}
	if err := abandonExpiredBackgroundTaskAttempts(q, tasks, 1234); err != nil {
		t.Fatalf("abandonExpiredBackgroundTaskAttempts: %v", err)
	}

	wantArgs := []int{899, 899, 5}
	if len(q.argCounts) != len(wantArgs) {
		t.Fatalf("Exec calls = %d, want %d", len(q.argCounts), len(wantArgs))
	}
	for i, got := range q.argCounts {
		if got != wantArgs[i] {
			t.Fatalf("Exec %d args = %d, want %d", i, got, wantArgs[i])
		}
		if got > maxVars {
			t.Fatalf("Exec %d uses %d binds, max %d", i, got, maxVars)
		}
	}
}
