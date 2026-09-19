package serve

import (
	"errors"
	"net/http"
	"strings"
)

func (s *Server) fileRouteHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/files/")
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || parts[0] == "" {
			s.handleFile(w, r)
			return
		}

		switch parts[1] {
		case "content", "download":
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
				return
			}
			s.handleOriginalMedia(w, r, parts[0], parts[1])
			return
		case "thumbnail", "preview", "lossless":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", "GET")
				writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
				return
			}
		}

		s.handleFile(w, r)
	})
}

func (s *Server) handleOriginalMedia(w http.ResponseWriter, r *http.Request, publicID string, route string) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	file, err := s.getFileByPublicID(r.Context(), publicID)
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load file", nil)
		return
	}
	s.media.serveOriginal(w, r, file, route == "download")
}
