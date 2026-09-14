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

func waitForTestBackgroundIdle(t *testing.T, client *core.Client) {
	t.Helper()

	changes, cancelChanges := client.SubscribeBackgroundOperationChanges()
	defer cancelChanges()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		active, err := client.CountActiveBackgroundOperations(false)
		if err != nil {
			t.Fatalf("count active background operations: %v", err)
		}
		if active == 0 {
			return
		}
		select {
		case <-changes:
		case <-timer.C:
			t.Fatalf("background runtime did not become idle; %d operations still active", active)
		}
	}
}
