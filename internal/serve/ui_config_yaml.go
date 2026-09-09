package serve

import "strings"

// UnmarshalYAML preserves the generated/default UI values while retaining whether grid_type
// was actually present in YAML. That lets booru-style choose a presentation-appropriate grid
// default without overriding an explicit user setting.
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
	if _, configured := fields["grid_type"]; !configured && strings.EqualFold(strings.TrimSpace(next.Theme), "booru-style") {
		next.GridType = "fit"
	}
	*cfg = UIConfig(next)
	return nil
}
