package gooru

import (
	"sort"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

type simpleUserTagFacetFilter struct {
	Tag     types.ParsedTag
	KeyOnly bool
}

// parseSimpleUserTagFacetFilter recognizes one positive, non-virtual user tag.
// Bare tags such as `hidden` are key-wide queries; explicit `key:value` terms
// are exact tag queries. Compound, negative, meta, ext:, type:, and wildcard
// expressions remain on the general query path.
func parseSimpleUserTagFacetFilter(expression string) (simpleUserTagFacetFilter, bool) {
	value := strings.TrimSpace(expression)
	ast, err := query.Parse(value)
	if err != nil || len(ast.Or) != 1 || len(ast.Or[0].And) != 1 {
		return simpleUserTagFacetFilter{}, false
	}
	term := ast.Or[0].And[0]
	if term.Not || term.Factor.SubExpr != nil || term.Factor.Tag == nil {
		return simpleUserTagFacetFilter{}, false
	}
	return simpleUserTagFacet(*term.Factor.Tag)
}

// parseSimpleUserTagFacetExclusions recognizes a pure conjunction of negative,
// non-virtual user tags such as `-"hidden" -"private:yes"`. This is the shape
// produced by the WebUI hidden-tag policy for an otherwise-root query. Mixed
// positive predicates, ORs, subexpressions, virtual/meta filters and wildcards
// remain on the general query path.
func parseSimpleUserTagFacetExclusions(expression string) ([]simpleUserTagFacetFilter, bool) {
	value := strings.TrimSpace(expression)
	ast, err := query.Parse(value)
	if err != nil || len(ast.Or) != 1 || len(ast.Or[0].And) == 0 {
		return nil, false
	}

	filters := make([]simpleUserTagFacetFilter, 0, len(ast.Or[0].And))
	for _, term := range ast.Or[0].And {
		if !term.Not || term.Factor.SubExpr != nil || term.Factor.Tag == nil {
			return nil, false
		}
		filter, ok := simpleUserTagFacet(*term.Factor.Tag)
		if !ok {
			return nil, false
		}
		filters = append(filters, filter)
	}
	return filters, true
}

func simpleUserTagFacet(tagStr string) (simpleUserTagFacetFilter, bool) {
	if strings.HasPrefix(tagStr, "@") || strings.Contains(tagStr, "*") {
		return simpleUserTagFacetFilter{}, false
	}
	parsed := query.ParseTag(tagStr)
	if parsed.Key == "ext" || parsed.Key == "type" {
		return simpleUserTagFacetFilter{}, false
	}
	return simpleUserTagFacetFilter{Tag: parsed, KeyOnly: !strings.Contains(tagStr, ":")}, true
}

func subtractKindFacets(all, excluded []types.TagWithCount) []types.TagWithCount {
	if len(all) == 0 {
		return nil
	}
	excludedByKind := make(map[string]int, len(excluded))
	for _, facet := range excluded {
		excludedByKind[facet.Tag] += facet.Count
	}

	remaining := make([]types.TagWithCount, 0, len(all))
	for _, facet := range all {
		facet.Count -= excludedByKind[facet.Tag]
		if facet.Count > 0 {
			remaining = append(remaining, facet)
		}
	}
	sort.Slice(remaining, func(i, j int) bool {
		if remaining[i].Count != remaining[j].Count {
			return remaining[i].Count > remaining[j].Count
		}
		return remaining[i].Tag < remaining[j].Tag
	})
	return remaining
}
