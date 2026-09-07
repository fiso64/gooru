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
	tagStr := *term.Factor.Tag
	if strings.HasPrefix(tagStr, "@") || strings.Contains(tagStr, "*") {
		return simpleUserTagFacetFilter{}, false
	}
	parsed := query.ParseTag(tagStr)
	if parsed.Key == "ext" || parsed.Key == "type" {
		return simpleUserTagFacetFilter{}, false
	}
	return simpleUserTagFacetFilter{Tag: parsed, KeyOnly: !strings.Contains(tagStr, ":")}, true
}
