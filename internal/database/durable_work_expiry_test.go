package database

import (
	"errors"
	"testing"
	"time"
)

func TestBackgroundTaskOutcomeRejectsExpiredLease(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		name string
		id   string
		act  func(string, time.Time) error
	}{
		{
			name: "complete",
			id:   "expired-complete",
			act: func(id string, finishedAt time.Time) error {
				return store.CompleteBackgroundTask(id, "worker-a", finishedAt)
			},
		},
		{
			name: "fail",
			id:   "expired-fail",
			act: func(id string, finishedAt time.Time) error {
				_, err := store.FailBackgroundTask(id, "worker-a", finishedAt, time.Time{}, "decode", "late failure")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
				ID: tc.id, DedupeKey: tc.id, Kind: "thumbnail", ResourceClass: "image", CreatedAt: now,
			}); err != nil || !created {
				t.Fatalf("enqueue = created %v, err %v", created, err)
			}
			claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
			if err != nil || !ok || claimed.ID != tc.id {
				t.Fatalf("claim = (%+v, %v, %v)", claimed, ok, err)
			}

			if err := tc.act(claimed.ID, now.Add(time.Minute)); !errors.Is(err, ErrBackgroundTaskLeaseLost) {
				t.Fatalf("expired outcome error = %v, want ErrBackgroundTaskLeaseLost", err)
			}

			var status, leaseOwner, attemptOutcome string
			if err := store.DB.QueryRow(`SELECT status, lease_owner FROM background_tasks WHERE id = ?`, claimed.ID).Scan(&status, &leaseOwner); err != nil {
				t.Fatal(err)
			}
			if err := store.DB.QueryRow(`SELECT outcome FROM background_task_attempts WHERE task_id = ? AND attempt_number = 1`, claimed.ID).Scan(&attemptOutcome); err != nil {
				t.Fatal(err)
			}
			if status != "running" || leaseOwner != "worker-a" || attemptOutcome != "running" {
				t.Fatalf("expired lease was mutated: status %q owner %q attempt %q", status, leaseOwner, attemptOutcome)
			}

			if recovered, err := store.RecoverExpiredBackgroundTaskLeases(now.Add(time.Minute)); err != nil || recovered != 1 {
				t.Fatalf("recover expired lease = %d, %v", recovered, err)
			}
		})
	}
}
