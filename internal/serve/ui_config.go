package serve

import "net/http"

type UIConfigResponse struct {
	AccentColor              string `json:"accent_color,omitempty"`
	FontStyle                string `json:"font_style"`
	LoadFullMediaByDefault   bool   `json:"load_full_media_by_default"`
	FullscreenMediaByDefault bool   `json:"fullscreen_media_by_default"`
	ViewerFitMode            string `json:"viewer_fit_mode"`
	GridSize                 int    `json:"grid_size"`
	ThumbnailSizes           []int  `json:"thumbnail_sizes"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, UIConfigResponse{
		AccentColor:              s.cfg.UI.AccentColor,
		FontStyle:                s.cfg.UI.FontStyle,
		LoadFullMediaByDefault:   s.cfg.UI.LoadFullMediaByDefault,
		FullscreenMediaByDefault: s.cfg.UI.FullscreenMediaByDefault,
		ViewerFitMode:            s.cfg.UI.ViewerFitMode,
		GridSize:                 s.cfg.UI.GridSize,
		ThumbnailSizes:           s.cfg.Media.ThumbnailSizes,
	})
}
