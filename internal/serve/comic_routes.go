package serve

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) handleComic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}

	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/comics/"), "/")
	parts := strings.Split(rest, "/")
	if rest == "" || len(parts) > 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "not_found", "comic not found", nil)
		return
	}

	file, err := s.getFileByPublicID(r.Context(), parts[0])
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "comic not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load comic", nil)
		return
	}

	if len(parts) == 2 {
		query := cloneURLValues(r.URL.Query())
		query.Set("page", parts[1])
		r.URL.RawQuery = query.Encode()
	}
	s.media.ServeComic(w, r, file, parts[0])
}

func cloneURLValues(values url.Values) url.Values {
	clone := make(url.Values, len(values))
	for key, items := range values {
		clone[key] = append([]string(nil), items...)
	}
	return clone
}
