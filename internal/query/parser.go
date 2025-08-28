package query

import (
	"strings"
)

// Parse takes a query expression and returns a list of tags.
// Placeholder.
func Parse(expression string) []string {
	parts := strings.Split(expression, "AND")
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}
