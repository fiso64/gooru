package serve

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	core "gooru.local/gooru"
)

func startTestBackgroundRuntime(t *testing.T, server *Server, client *core.Client, workerID string) func() {
	t.Helper()

	runtime, err := server.NewBackgroundRuntime(client, workerID)
	if err != nil {
		t.Fatalf("background runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case runErr := <-done:
				if runErr != nil && !errors.Is(runErr, context.Canceled) {
					t.Errorf("background runtime shutdown: %v", runErr)
				}
			case <-time.After(2 * time.Second):
				t.Error("background runtime did not stop after cancellation")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

// waitForTestBackgroundIdle waits for the registration-triggered media metadata
// backlog to drain. The upload operation itself has completed before current
// callers enter this helper, so pending metadata is the remaining durable work
// they need to synchronize with before asserting cached values.
func waitForTestBackgroundIdle(t *testing.T, client *core.Client) {
	t.Helper()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		pending, err := client.ListPendingMediaMetadataFiles(0, 1)
		if err != nil {
			t.Fatalf("list pending media metadata: %v", err)
		}
		if len(pending) == 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatalf("background runtime did not process pending media metadata for location %d", pending[0].ID)
		}
	}
}