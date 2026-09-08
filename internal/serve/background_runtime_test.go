package serve

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type backgroundRuntimeFunc func(context.Context) error

func (fn backgroundRuntimeFunc) Run(ctx context.Context) error {
	return fn(ctx)
}

func TestListenAndServeWithBackgroundRuntimeCancelsRuntimeOnShutdown(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.Listen = "127.0.0.1:0"
	server := NewServer(cfg)
	started := make(chan struct{})
	stopped := make(chan struct{})
	runtime := backgroundRuntimeFunc(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServeWithBackgroundRuntime(ctx, runtime) }()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("background runtime did not start")
	}
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("shutdown returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("background runtime was not stopped before server returned")
	}
}

func TestListenAndServeWithBackgroundRuntimePropagatesRuntimeFailure(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.Listen = "127.0.0.1:0"
	server := NewServer(cfg)
	wantErr := errors.New("background worker failed")
	runtime := backgroundRuntimeFunc(func(context.Context) error {
		return wantErr
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.ListenAndServeWithBackgroundRuntime(ctx, runtime); !errors.Is(err, wantErr) {
		t.Fatalf("expected runtime error %v, got %v", wantErr, err)
	}
}

func TestListenAndServeWithBackgroundRuntimeRejectsUnexpectedCleanExit(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Server.Listen = "127.0.0.1:0"
	server := NewServer(cfg)
	runtime := backgroundRuntimeFunc(func(context.Context) error { return nil })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := server.ListenAndServeWithBackgroundRuntime(ctx, runtime)
	if err == nil || !strings.Contains(err.Error(), "background runtime stopped unexpectedly") {
		t.Fatalf("expected unexpected-runtime-exit error, got %v", err)
	}
}
