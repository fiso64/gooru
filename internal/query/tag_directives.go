package query

import (
	"fmt"
	"strings"
)

// ValidateTagDirective validates an upload tag directive. A leading dash means
// the corresponding positive tag should be excluded from the resolved set.
func ValidateTagDirective(directive string) error {
	directive = strings.TrimSpace(directive)
	if directive == "" {
		return fmt.Errorf("tag directive cannot be empty")
	}
	tag := directive
	if strings.HasPrefix(tag, "-") {
		tag = strings.TrimPrefix(tag, "-")
		if tag == "" {
			return fmt.Errorf("tag exclusion cannot be empty")
		}
	}
	return ValidateTag(tag)
}

func ValidateTagDirectives(directives []string) error {
	for _, directive := range directives {
		if err := ValidateTagDirective(directive); err != nil {
			return err
		}
	}
	return nil
}

// ResolveTagDirectives applies tag/-tag directives in order and returns the
// positive tags to apply. Exclusions remove an earlier matching tag; a later
// positive directive can add it again.
func ResolveTagDirectives(directives []string) ([]string, error) {
	if err := ValidateTagDirectives(directives); err != nil {
		return nil, err
	}
	result := make([]string, 0, len(directives))
	present := make(map[string]struct{}, len(directives))
	for _, raw := range directives {
		directive := strings.TrimSpace(raw)
		if strings.HasPrefix(directive, "-") {
			tag := strings.TrimPrefix(directive, "-")
			if _, ok := present[tag]; !ok {
				continue
			}
			delete(present, tag)
			filtered := result[:0]
			for _, existing := range result {
				if existing != tag {
					filtered = append(filtered, existing)
				}
			}
			result = filtered
			continue
		}
		if _, ok := present[directive]; ok {
			continue
		}
		present[directive] = struct{}{}
		result = append(result, directive)
	}
	return result, nil
}
