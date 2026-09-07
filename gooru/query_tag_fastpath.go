package gooru

import (
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// simpleExactUserTagFilter recognizes one positive, non-virtual exact user tag.
// It deliberately mirrors the existing count fast path and declines compound,
// negative, meta, ext:, type:, wildcard, and otherwise general expressions.
func simpleExactUserTagFilter(expression string) (types.ParsedTag, bool) {
	value := strings.TrimSpace(expression)
	ast, err := query.Parse(value)
	if err != nil || len(ast.Or) != 1 || len(ast.Or[0].And) != 1 {
		return types.ParsedTag{}, false
	}
	term := ast.Or[0].And[0]
	if term.Not || term.Factor.SubExpr != nil || term.Factor.Tag == nil {
		return types.ParsedTag{}, false
	}
	tagStr := *term.Factor.Tag
	if strings.HasPrefix(tagStr, "@") {
		return types.ParsedTag{}, false
	}
	parsed := query.ParseTag(tagStr)
	if parsed.Key == "ext" || parsed.Key == "type" || (parsed.Value == "" && !strings.HasSuffix(tagStr, ":")) {
		return types.ParsedTag{}, false
	}
	return parsed, true
}
