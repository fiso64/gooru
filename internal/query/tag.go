package query

import (
	"strings"

	"gooru.local/types"
)

// ParseTag parses a tag string (e.g., "key:value" or "value") into a ParsedTag.
// If a colon is present, it's a key:value pair.
// Otherwise, the entire string is the key, and the value is empty.
func ParseTag(tag string) types.ParsedTag {
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) == 2 {
		// Handles "key:value" and "key:" (value is empty string)
		return types.ParsedTag{Key: parts[0], Value: parts[1]}
	}
	// Handles "key" (simple tag)
	return types.ParsedTag{Key: tag, Value: ""}
}

// ParseTags parses a collection of tag strings while preserving caller order.
func ParseTags(tags []string) []types.ParsedTag {
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, tag := range tags {
		parsedTags[i] = ParseTag(tag)
	}
	return parsedTags
}
