package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// BackgroundRuntime is the narrow server composition contract for durable work.
// Persistence, task types, and handler registration remain owned outside the
// HTTP package.
type BackgroundRuntime interface {
	Run(context.Context) error
}

type multiBackgroundRuntime struct {
	runtimes []BackgroundRuntime
}

func (m multiBackgroundRuntime) Run(ctx context.Context) error {
	if len(m.runtimes) == 0 {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtime := runtime
		go func() { errs <- runtime.Run(runCtx) }()
	}
	first := <-errs
	cancel()
	joined := first
	for i := 1; i < len(m.runtimes); i++ {
		err := <-errs
		if errors.Is(err, context.Canceled) {
			err = nil
		}
		joined = errors.Join(joined, err)
	}
	return joined
}

// ListenAndServeWithBackgroundRuntime owns the HTTP server and durable worker as
// one process lifecycle. Failure of either side cancels the other, while normal
// process cancellation waits for both sides to stop before returning.
func (s *Server) ListenAndServeWithBackgroundRuntime(ctx context.Context, runtime BackgroundRuntime) error {
	if runtime == nil {
		return s.ListenAndServe(ctx)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	server := s.HTTPServer()
	httpErrCh := make(chan error, 1)
	runtimeErrCh := make(chan error, 1)
	go func() { httpErrCh <- server.ListenAndServe() }()
	go func() { runtimeErrCh <- runtime.Run(runCtx) }()

	select {
	case <-ctx.Done():
		cancel()
		shutdownErr := shutdownHTTPServer(server)
		httpErr := normalizeHTTPServerError(<-httpErrCh)
		runtimeErr := normalizeCanceledRuntimeError(<-runtimeErrCh, ctx)
		return errors.Join(shutdownErr, httpErr, runtimeErr)
	case runtimeErr := <-runtimeErrCh:
		cancel()
		shutdownErr := shutdownHTTPServer(server)
		httpErr := normalizeHTTPServerError(<-httpErrCh)
		if runtimeErr == nil && ctx.Err() == nil {
			runtimeErr = errors.New("background runtime stopped unexpectedly")
		}
		return errors.Join(runtimeErr, shutdownErr, httpErr)
	case httpErr := <-httpErrCh:
		cancel()
		runtimeErr := <-runtimeErrCh
		httpErr = normalizeHTTPServerError(httpErr)
		if httpErr == nil && ctx.Err() == nil {
			httpErr = errors.New("http server stopped unexpectedly")
		}
		if errors.Is(runtimeErr, context.Canceled) {
			runtimeErr = nil
		}
		return errors.Join(httpErr, runtimeErr)
	}
}

func shutdownHTTPServer(server *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	return nil
}

func normalizeHTTPServerError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func normalizeCanceledRuntimeError(err error, parent context.Context) error {
	if parent.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
