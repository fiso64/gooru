package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type writeDeadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (w *writeDeadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func TestRequestWriteTimeoutRefreshesBeforeEachResponseWrite(t *testing.T) {
	recorder := &writeDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	timeout := time.Minute
	started := time.Now()

	handler := requestWriteTimeoutMiddleware(timeout, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if len(recorder.deadlines) != 2 {
		t.Fatalf("expected deadline refresh before header and body writes, got %d", len(recorder.deadlines))
	}
	for i, deadline := range recorder.deadlines {
		if !deadline.After(started) {
			t.Fatalf("deadline %d was not refreshed into the future: %v", i, deadline)
		}
		if deadline.After(time.Now().Add(timeout + time.Second)) {
			t.Fatalf("deadline %d exceeds expected timeout window: %v", i, deadline)
		}
	}
}

func TestRequestWriteTimeoutAllowsStreamBeyondAbsoluteServerDeadline(t *testing.T) {
	const timeout = 50 * time.Millisecond
	writeErrors := make(chan error, 2)
	handler := requestWriteTimeoutMiddleware(timeout, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		controller := http.NewResponseController(w)
		if _, err := w.Write([]byte("first")); err != nil {
			writeErrors <- err
			return
		}
		if err := controller.Flush(); err != nil {
			writeErrors <- err
			return
		}

		// Cross the server's original absolute WriteTimeout. The second write
		// must refresh the deadline rather than inheriting the expired one.
		time.Sleep(3 * timeout)
		if _, err := w.Write([]byte("second")); err != nil {
			writeErrors <- err
			return
		}
		if err := controller.Flush(); err != nil {
			writeErrors <- err
		}
	}))

	server := httptest.NewUnstartedServer(handler)
	server.Config.WriteTimeout = timeout
	server.Start()
	defer server.Close()

	client := server.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("get streaming response: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read streaming response: %v", err)
	}
	select {
	case err := <-writeErrors:
		t.Fatalf("stream write failed: %v", err)
	default:
	}
	if got := string(body); got != "firstsecond" {
		t.Fatalf("expected complete streaming response, got %q", got)
	}
}

func TestRequestWriteTimeoutDisabledLeavesResponseWriterUntouched(t *testing.T) {
	recorder := &writeDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	handler := requestWriteTimeoutMiddleware(0, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if len(recorder.deadlines) != 0 {
		t.Fatalf("disabled timeout should not set write deadlines, got %d", len(recorder.deadlines))
	}
}

func TestRefreshingWriteDeadlineResponseWriterPreservesControllerAccess(t *testing.T) {
	recorder := &writeDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	wrapped := &refreshingWriteDeadlineResponseWriter{
		ResponseWriter: recorder,
		timeout:        time.Minute,
	}

	if err := http.NewResponseController(wrapped).SetWriteDeadline(time.Time{}); err != nil {
		t.Fatalf("clear write deadline through wrapper: %v", err)
	}
	if len(recorder.deadlines) != 1 || !recorder.deadlines[0].IsZero() {
		t.Fatalf("expected controller deadline clear to reach underlying writer, got %+v", recorder.deadlines)
	}
}
