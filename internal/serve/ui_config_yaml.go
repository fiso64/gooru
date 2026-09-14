package serve

import "strings"

const defaultBooruGridSize = 180

func isBooruUITheme(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "booru-light", "booru-dark":
		return true
	default:
		return false
	}
}

// UnmarshalYAML preserves the generated/default UI values while retaining whether
// theme-sensitive settings were actually present in YAML. Booru themes can therefore
// choose their own defaults without overriding explicit user settings.
func (cfg *UIConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plainUIConfig UIConfig
	next := plainUIConfig(*cfg)
	if err := unmarshal(&next); err != nil {
		return err
	}

	var fields map[string]interface{}
	if err := unmarshal(&fields); err != nil {
		return err
	}
	_, next.FontStyleConfigured = fields["font_style"]
	if isBooruUITheme(next.Theme) {
		if _, configured := fields["grid_type"]; !configured {
			next.GridType = "fit"
		}
		if _, configured := fields["grid_size"]; !configured {
			next.GridSize = defaultBooruGridSize
		}
		if _, configured := fields["pagination_mode"]; !configured {
			next.PaginationMode = "paged"
		}
	}
	*cfg = UIConfig(next)
	return nil
}
