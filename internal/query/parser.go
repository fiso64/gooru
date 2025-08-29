package query

import (
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// AST Definitions for a grammar with proper operator precedence.

// Expression is a series of AndTerms separated by OR operators.
// E.g. "a & b | c"
type Expression struct {
	Or []*AndTerm `@@ ("|" @@)*`
}

// AndTerm is a series of Terms separated by AND operators (explicit or implicit).
// E.g. "a -b c"
type AndTerm struct {
	And []*Term `@@+`
}

// Term is a Factor that can be negated.
// E.g. "-tag" or "tag"
type Term struct {
	Not    bool    `@"-"?`
	Factor *Factor `@@`
}

// Factor is the base unit: a tag or a grouped sub-expression.
type Factor struct {
	Tag     *string     `@Tag | @QuotedString`
	SubExpr *Expression `| "(" @@ ")"`
}

// Parser

var (
	queryLexer = lexer.MustSimple([]lexer.SimpleRule{
		{Name: "QuotedString", Pattern: `"(\\"|[^"])*"`},
		// A tag cannot start with a hyphen to avoid ambiguity with the NOT operator.
		{Name: "Tag", Pattern: `[a-zA-Z0-9_./\\][a-zA-Z0-9_./\\:-]*`},
		{Name: "Operator", Pattern: `[|()&-]`},
		{Name: "Whitespace", Pattern: `\s+`},
	})

	parser = participle.MustBuild[Expression](
		participle.Lexer(queryLexer),
		participle.Unquote("QuotedString"),
		participle.Elide("Whitespace"),
		// Use an explicit "&" token in the grammar if you want to support it,
		// but implicit AND (a sequence of terms) is often sufficient and cleaner.
		// The current grammar `@@+` handles implicit AND.
	)
)

// Parse takes a query expression string and returns the parsed AST.
func Parse(expression string) (*Expression, error) {
	// Participle's parser doesn't consume "&" as an operator, it treats it as a tag.
	// We can manually replace it with a space to support it as an implicit AND separator.
	// A more advanced grammar could handle it, but this is a simple and effective solution.
	expression = strings.ReplaceAll(expression, "&", " ")
	return parser.ParseString("", expression)
}