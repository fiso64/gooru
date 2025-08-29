package query

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// AST Definitions

// Expression represents an OR operation.
// E.g., "a | b | c"
type Expression struct {
	Left  *AndExpression   `@@`
	Right []*OpAndExpression `@@*`
}

type OpAndExpression struct {
	Operator string         `@("|")`
	Right    *AndExpression `@@`
}

// AndExpression represents an AND or EXCEPT operation.
// E.g., "a b", "a & b", "a - b"
type AndExpression struct {
	Left  *Term   `@@?` // Optional to allow leading NOT, e.g. "-tag"
	Right []*OpTerm `@@*`
}

type OpTerm struct {
	// Operator is optional for implicit AND. It can be "&" or "-".
	Operator string `@( "&" | "-" )?`
	Right    *Term  `@@`
}

// Term is a single unit in an expression.
type Term struct {
	// A term can be a tag, a quoted string, or a sub-expression in parentheses.
	Tag     *string      `@Tag | @QuotedString`
	SubExpr *Expression `| "(" @@ ")"`
}

// Parser

var (
	queryLexer = lexer.MustSimple([]lexer.SimpleRule{
		{Name: "QuotedString", Pattern: `"(\\"|[^"])*"`},
		// A tag can contain colons and hyphens, but importantly, cannot START with a hyphen.
		// This ensures that a leading "-" is always parsed as a NOT operator.
		{Name: "Tag", Pattern: `[a-zA-Z0-9_./\\][a-zA-Z0-9_./\\:-]*`},
		{Name: "Operator", Pattern: `[|&\-()]`},
		{Name: "Whitespace", Pattern: `\s+`},
	})

	parser = participle.MustBuild[Expression](
		participle.Lexer(queryLexer),
		participle.Unquote("QuotedString"),
		// The grammar with optional operators correctly handles implicit AND.
		participle.Elide("Whitespace"),
	)
)

// Parse takes a query expression string and returns the parsed AST.
func Parse(expression string) (*Expression, error) {
	return parser.ParseString("", expression)
}