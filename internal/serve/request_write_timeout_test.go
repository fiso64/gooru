package serve

import (
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
