package serve

import "net/http"

type backgroundOperationActiveLister interface {
	ListActiveBackgroundOperationIDs(bool) ([]string, error)
}

type BackgroundOperationCancelAllResponse struct {
	Canceled int `json:"canceled"`
}

func (l *GooruLibrary) ListActiveBackgroundOperationIDs(visibleOnly bool) ([]string, error) {
	return l.client.ListActiveBackgroundOperationIDs(visibleOnly)
}

func (s *Server) handleCancelAllOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodDelete)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.backgroundOperations == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
		return
	}
	lister, ok := s.backgroundOperations.(backgroundOperationActiveLister)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "active background operation listing is not configured", nil)
		return
	}

	ids, err := lister.ListActiveBackgroundOperationIDs(true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list active background operations", nil)
		return
	}
	canceled := 0
	for _, id := range ids {
		ok, err := s.cancelBackgroundOperation(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to cancel active background operations", nil)
			return
		}
		if ok {
			canceled++
		}
	}
	writeJSON(w, http.StatusOK, BackgroundOperationCancelAllResponse{Canceled: canceled})
}
