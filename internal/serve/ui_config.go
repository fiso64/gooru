package serve

import "net/http"

type UIConfigResponse struct {
	AccentColor            string `json:"accent_color,omitempty"`
	FontStyle              string `json:"font_style"`
	LoadFullMediaByDefault bool   `json:"load_full_media_by_default"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, UIConfigResponse{
		AccentColor:            s.cfg.UI.AccentColor,
		FontStyle:              s.cfg.UI.FontStyle,
		LoadFullMediaByDefault: s.cfg.Media.LoadFullByDefault,
	})
}
