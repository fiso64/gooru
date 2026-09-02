package serve

import "net/http"

type UIConfigResponse struct {
	AccentColor            string `json:"accent_color,omitempty"`
	LoadFullMediaByDefault bool   `json:"load_full_media_by_default"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, UIConfigResponse{
		AccentColor:            s.cfg.UI.AccentColor,
		LoadFullMediaByDefault: s.cfg.Media.LoadFullByDefault,
	})
}
