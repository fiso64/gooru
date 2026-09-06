package gooru

import (
	"strings"

	"gooru.local/types"
)

// simpleKindFacetFilter recognizes the exact kind filters emitted by the
// library sidebar. Keeping this deliberately narrow avoids changing semantics
// for compound, negative, wildcard, legacy alias, or otherwise general queries.
func simpleKindFacetFilter(expression string) (string, bool) {
	value := strings.TrimSpace(expression)
	if !strings.HasPrefix(value, "type:") || strings.ContainsAny(value, " \t\r\n|&!()-*\"") {
		return "", false
	}
	kind := strings.ToLower(strings.TrimPrefix(value, "type:"))
	switch kind {
	case "photo", "video", "gif", "audio", "comic", "other":
		return kind, true
	default:
		return "", false
	}
}

func filterKindFacet(facets []types.TagWithCount, kind string) []types.TagWithCount {
	for _, facet := range facets {
		if facet.Tag == kind {
			return []types.TagWithCount{facet}
		}
	}
	return []types.TagWithCount{}
}
