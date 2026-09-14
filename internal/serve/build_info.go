package serve

import (
	"gooru.local/internal/buildinfo"
	"net/http"
)

func (s *Server) handleBuildInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildinfo.Current())
}
