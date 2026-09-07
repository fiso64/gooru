package gooru

import (
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

type simpleUserTagFacetFilter struct {
	Tag     types.ParsedTag
	KeyOnly bool
}

func parseSimpleUserTagFacetTerm(termTag string) (simpleUserTagFacetFilter, bool) {
	if strings.HasPrefix(termTag, "@") || strings.Contains(termTag, "*") {
		return simpleUserTagFacetFilter{}, false
	}
	parsed := query.ParseTag(termTag)
	if parsed.Key == "ext" || parsed.Key == "type" {
		return simpleUserTagFacetFilter{}, false
	}
	return simpleUserTagFacetFilter{Tag: parsed, KeyOnly: !strings.Contains(termTag, ":")}, true
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
	return parseSimpleUserTagFacetTerm(*term.Factor.Tag)
}

// parseSimpleUserTagFacetExclusions recognizes a single AND group made only of
// negative, non-virtual user tags. This is the shape produced by the WebUI's
// hidden-tag baseline. Compound user queries remain on the general query path.
func parseSimpleUserTagFacetExclusions(expression string) ([]simpleUserTagFacetFilter, bool) {
	value := strings.TrimSpace(expression)
	ast, err := query.Parse(value)
	if err != nil || len(ast.Or) != 1 || len(ast.Or[0].And) == 0 {
		return nil, false
	}
	filters := make([]simpleUserTagFacetFilter, 0, len(ast.Or[0].And))
	for _, term := range ast.Or[0].And {
		if term == nil || !term.Not || term.Factor == nil || term.Factor.SubExpr != nil || term.Factor.Tag == nil {
			return nil, false
		}
		filter, ok := parseSimpleUserTagFacetTerm(*term.Factor.Tag)
		if !ok {
			return nil, false
		}
		filters = append(filters, filter)
	}
	return filters, true
}
