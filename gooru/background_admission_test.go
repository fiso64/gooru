package gooru

import (
	"errors"
	"sync"
	"testing"
)

func TestCreateBackgroundOperationWithPendingLimitRejectsAtLimit(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	request := BackgroundOperationRequest{Kind: "upload-import", Visible: true, ProgressTotal: 1}

	first, err := client.CreateBackgroundOperationWithPendingLimit(request, 1)
	if err != nil {
		t.Fatalf("create first bounded operation: %v", err)
	}
	if first.ID == "" {
		t.Fatal("first bounded operation has empty id")
	}

	if _, err := client.CreateBackgroundOperationWithPendingLimit(request, 1); !errors.Is(err, ErrBackgroundOperationPendingLimit) {
		t.Fatalf("second bounded operation error = %v, want %v", err, ErrBackgroundOperationPendingLimit)
	}
}

func TestCreateBackgroundOperationWithPendingLimitReleasesCapacityWhenRunning(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	request := BackgroundOperationRequest{Kind: "upload-import", Visible: true, ProgressTotal: 1}

	first, err := client.CreateBackgroundOperationWithPendingLimit(request, 1)
	if err != nil {
		t.Fatalf("create first bounded operation: %v", err)
	}
	if _, err := client.store.DB.Exec(`UPDATE background_operations SET status = 'running' WHERE id = ?`, first.ID); err != nil {
		t.Fatalf("mark first operation running: %v", err)
	}
	if _, err := client.CreateBackgroundOperationWithPendingLimit(request, 1); err != nil {
		t.Fatalf("create operation after pending slot released: %v", err)
	}
}

func TestCreateBackgroundOperationWithPendingLimitScopesCapacityByKind(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	if _, err := client.CreateBackgroundOperationWithPendingLimit(BackgroundOperationRequest{Kind: "upload-import"}, 1); err != nil {
		t.Fatalf("create upload operation: %v", err)
	}
	if _, err := client.CreateBackgroundOperationWithPendingLimit(BackgroundOperationRequest{Kind: "other-work"}, 1); err != nil {
		t.Fatalf("create different-kind operation: %v", err)
	}
}

func TestCreateBackgroundOperationWithPendingLimitIsAtomicAcrossConcurrentProducers(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	request := BackgroundOperationRequest{Kind: "upload-import"}

	const producers = 8
	var wg sync.WaitGroup
	wg.Add(producers)
	results := make(chan error, producers)
	for i := 0; i < producers; i++ {
		go func() {
			defer wg.Done()
			_, err := client.CreateBackgroundOperationWithPendingLimit(request, 1)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	succeeded := 0
	limited := 0
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrBackgroundOperationPendingLimit):
			limited++
		default:
			t.Fatalf("unexpected admission error: %v", err)
		}
	}
	if succeeded != 1 || limited != producers-1 {
		t.Fatalf("admission results: succeeded=%d limited=%d, want 1 and %d", succeeded, limited, producers-1)
	}
}
