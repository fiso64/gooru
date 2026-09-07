package serve

import "net/http"

const UICapabilityPreviewImages = "preview_images"

type UIConfigResponse struct {
	AccentColor              string   `json:"accent_color,omitempty"`
	FontStyle                string   `json:"font_style"`
	LoadFullMediaByDefault   bool     `json:"load_full_media_by_default"`
	FullscreenMediaByDefault bool     `json:"fullscreen_media_by_default"`
	ViewerFitMode            string   `json:"viewer_fit_mode"`
	ViewerScaling            string   `json:"viewer_scaling"`
	GridSize                 int      `json:"grid_size"`
	GridType                 string   `json:"grid_type"`
	PaginationMode           string   `json:"pagination_mode"`
	ItemsPerPage             int      `json:"items_per_page"`
	ThumbnailSizes           []int    `json:"thumbnail_sizes"`
	ProtectedMode            bool     `json:"protected_mode"`
	OpaqueURLState           bool     `json:"opaque_url_state"`
	Capabilities             []string `json:"capabilities"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	capabilities := make([]string, 0, 1)
	if s.cfg.Media.PreviewEnabled {
		capabilities = append(capabilities, UICapabilityPreviewImages)
	}
	writeJSON(w, http.StatusOK, UIConfigResponse{
		AccentColor:              s.cfg.UI.AccentColor,
		FontStyle:                s.cfg.UI.FontStyle,
		LoadFullMediaByDefault:   s.cfg.UI.LoadFullMediaByDefault,
		FullscreenMediaByDefault: s.cfg.UI.FullscreenMediaByDefault,
		ViewerFitMode:            s.cfg.UI.ViewerFitMode,
		ViewerScaling:            s.cfg.UI.ViewerScaling,
		GridSize:                 s.cfg.UI.GridSize,
		GridType:                 s.cfg.UI.GridType,
		PaginationMode:           s.cfg.UI.PaginationMode,
		ItemsPerPage:             s.cfg.UI.ItemsPerPage,
		ThumbnailSizes:           s.cfg.Media.ThumbnailSizes,
		ProtectedMode:            s.cfg.Encryption.Enabled,
		OpaqueURLState:           s.cfg.Encryption.Enabled && s.cfg.Encryption.OpaqueURLState,
		Capabilities:             capabilities,
	})
}
