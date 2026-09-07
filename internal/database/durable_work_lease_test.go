package database

import (
	"errors"
	"testing"
	"time"
)

func TestRenewBackgroundTaskLeaseExtendsOnlyCurrentValidOwner(t *testing.T) {
	store := newDurableLifecycleTestStore(t)
	now := time.Date(2026, time.September, 7, 23, 0, 0, 0, time.UTC)
	if _, created, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{
		ID: "renew-task", DedupeKey: "renew", Kind: "thumbnail", ResourceClass: "image", CreatedAt: now,
	}); err != nil || !created {
		t.Fatalf("enqueue = created %v, err %v", created, err)
	}
	claimed, ok, err := store.ClaimNextBackgroundTask("image", "worker-a", now, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim = (%+v, %v, %v)", claimed, ok, err)
	}

	renewedAt := now.Add(30 * time.Second)
	expiresAt, err := store.RenewBackgroundTaskLease(claimed.ID, "worker-a", renewedAt, 2*time.Minute)
	if err != nil {
		t.Fatalf("renew current owner: %v", err)
	}
	if !expiresAt.Equal(renewedAt.Add(2 * time.Minute)) {
		t.Fatalf("renewed expiry = %v, want %v", expiresAt, renewedAt.Add(2*time.Minute))
	}
	if _, err := store.RenewBackgroundTaskLease(claimed.ID, "worker-b", renewedAt, time.Minute); !errors.Is(err, ErrBackgroundTaskLeaseLost) {
		t.Fatalf("wrong owner renewal error = %v, want lease lost", err)
	}

	if _, err := store.RenewBackgroundTaskLease(claimed.ID, "worker-a", expiresAt, time.Minute); !errors.Is(err, ErrBackgroundTaskLeaseLost) {
		t.Fatalf("expired lease renewal error = %v, want lease lost", err)
	}
	if recovered, err := store.RecoverExpiredBackgroundTaskLeases(expiresAt); err != nil || recovered != 1 {
		t.Fatalf("recovery after refused late renewal = %d, %v", recovered, err)
	}
}
