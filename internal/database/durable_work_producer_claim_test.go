package database

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func newProducerClaimTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "gooru.db"), false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := RunMigrations(store.DB); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return store
}

func TestClaimBackgroundOperationProducerAllowsExactlyOneConcurrentClaim(t *testing.T) {
	store := newProducerClaimTestStore(t)
	op, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true})
	if err != nil {
		t.Fatal(err)
	}

	var wins atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, err := store.ClaimBackgroundOperationProducer(op.ID)
			if err != nil {
				errs <- err
				return
			}
			if claimed {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("claim producer: %v", err)
	}
	if got := wins.Load(); got != 1 {
		t.Fatalf("successful producer claims = %d, want 1", got)
	}
}

func TestClaimBackgroundOperationProducerRejectsAttachedOperation(t *testing.T) {
	store := newProducerClaimTestStore(t)
	op, err := store.CreateBackgroundOperation(store.DB, NewBackgroundOperation{ID: "upload-op", Kind: "upload_import", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EnqueueBackgroundTask(store.DB, NewBackgroundTask{ID: "existing", OperationID: op.ID, DedupeKey: "existing", Kind: "upload.import"}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimBackgroundOperationProducer(op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("attached operation unexpectedly accepted a producer claim")
	}
}
