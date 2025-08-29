package query

import (
	"strings"
)

// SQLBuilder transforms a query AST into a SQL query string and arguments.
type SQLBuilder struct {
	query strings.Builder
	args  []interface{}
}

// Build generates the SQL query from the AST.
func Build(expr *Expression) (string, []interface{}) {
	if expr == nil || len(expr.Or) == 0 {
		return "", nil
	}
	b := &SQLBuilder{}
	b.buildExpression(expr)
	return b.query.String(), b.args
}

// buildExpression handles OR nodes (UNION).
func (b *SQLBuilder) buildExpression(expr *Expression) {
	for i, andTerm := range expr.Or {
		if i > 0 {
			b.query.WriteString(" UNION ")
		}
		b.buildAndTerm(andTerm)
	}
}

// buildAndTerm handles AND nodes (INTERSECT).
func (b *SQLBuilder) buildAndTerm(andTerm *AndTerm) {
	for i, term := range andTerm.And {
		if i > 0 {
			b.query.WriteString(" INTERSECT ")
		}
		b.buildTerm(term)
	}
}

// buildTerm handles NOT nodes (EXCEPT).
func (b *SQLBuilder) buildTerm(term *Term) {
	if term.Not {
		// A negated term `U - A` must be parenthesized to ensure correct
		// precedence when used in a larger compound statement.
		b.query.WriteString("(")
		b.query.WriteString("SELECT hash FROM contents EXCEPT ")
		b.buildFactor(term.Factor)
		b.query.WriteString(")")
	} else {
		b.buildFactor(term.Factor)
	}
}

// buildFactor handles the base cases: a tag or a sub-expression.
func (b *SQLBuilder) buildFactor(factor *Factor) {
	if factor.SubExpr != nil {
		// A user-defined group must be converted into a valid SELECT statement
		// (a derived table) so it can be legally used as an operand for
		// operators like INTERSECT. Adding `AS t` provides a required alias.
		b.query.WriteString("(SELECT hash FROM (")
		b.buildExpression(factor.SubExpr)
		b.query.WriteString(") AS t)")
	} else if factor.Tag != nil {
		b.buildTagQuery(*factor.Tag)
	}
}

// buildTagQuery generates the base SELECT statement for a single tag.
func (b *SQLBuilder) buildTagQuery(tagStr string) {
	parsed := ParseTag(tagStr)
	// A simple SELECT is a valid operand and does not need wrapping.
	b.query.WriteString(
		`SELECT ct.content_hash as hash FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE t.key = ? AND t.value = ?`,
	)
	b.args = append(b.args, parsed.Key, parsed.Value)
}