package serve

import (
	"io"
	"net/http"
	"time"
)

const backgroundOperationEventKeepaliveInterval = 15 * time.Second

type backgroundOperationChangeSubscriber interface {
	SubscribeBackgroundOperationChanges() (<-chan struct{}, func())
}

func (l *GooruLibrary) SubscribeBackgroundOperationChanges() (<-chan struct{}, func()) {
	return l.client.SubscribeBackgroundOperationChanges()
}

func (s *Server) handleOperationEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	subscriber, ok := s.backgroundOperations.(backgroundOperationChangeSubscriber)
	if !ok || subscriber == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation events are not configured", nil)
		return
	}

	controller := http.NewResponseController(w)
	// SSE connections intentionally outlive the server's ordinary response write
	// timeout. ResponseController traverses the logging wrapper via Unwrap.
	if err := controller.SetWriteDeadline(time.Time{}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "background operation event streaming is not supported", nil)
		return
	}

	changes, unsubscribe := subscriber.SubscribeBackgroundOperationChanges()
	defer unsubscribe()

	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	if err := writeBackgroundOperationEvent(w); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}

	keepalive := time.NewTicker(backgroundOperationEventKeepaliveInterval)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, open := <-changes:
			if !open {
				return
			}
			if err := writeBackgroundOperationEvent(w); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		case <-keepalive.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			if err := controller.Flush(); err != nil {
				return
			}
		}
	}
}

func writeBackgroundOperationEvent(w io.Writer) error {
	_, err := io.WriteString(w, "event: operations\ndata:\n\n")
	return err
}
