package serve

import (
	"context"
	"errors"
	"testing"
	"time"

	core "gooru.local/gooru"
)

func startTestBackgroundRuntime(t *testing.T, server *Server, client *core.Client, workerID string) {
	t.Helper()

	runtime, err := server.NewBackgroundRuntime(client, workerID)
	if err != nil {
		t.Fatalf("background runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()

	t.Cleanup(func() {
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
