package serve

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTPServerShutdownCancelsOperationEventStream(t *testing.T) {
	store := &operationEventTestStore{changes: make(chan struct{}, 1)}
	server := (&Server{backgroundOperations: store}).HTTPServer()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(listener)
	}()

	response, err := http.Get("http://" + listener.Addr().String() + "/api/v1/operations/events")
	if err != nil {
		t.Fatalf("open operation event stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	assertOperationEvent(t, bufio.NewReader(response.Body))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown with active operation event stream: %v", err)
	}

	select {
	case err := <-serveDone:
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("serve error = %v, want http.ErrServerClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after shutdown")
	}
}
