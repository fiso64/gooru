package query

import (
	"gooru.local/gooru/internal/types"
	"strings"
)

// ParseTag parses a tag string (e.g., "key:value" or "value") into a ParsedTag.
// A tag is considered key-value if it contains a non-empty key before the first colon.
// Otherwise, it's a simple tag where the whole string is the value.
func ParseTag(tag string) types.ParsedTag {
	parts := strings.SplitN(tag, ":", 2)
	// A key-value tag must have a non-empty key.
	if len(parts) == 2 && parts[0] != "" {
		return types.ParsedTag{Key: parts[0], Value: parts[1]}
	}
	// Otherwise, it's a simple tag, and the whole string is the value.
	return types.ParsedTag{Key: "", Value: tag}
}