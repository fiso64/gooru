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
	if expr == nil {
		return "", nil
	}
	b := &SQLBuilder{}
	b.buildExpression(expr)
	return b.query.String(), b.args
}

// buildExpression handles OR nodes (UNION).
func (b *SQLBuilder) buildExpression(expr *Expression) {
	if expr == nil || expr.Left == nil {
		return
	}

	isSubQuery := len(expr.Right) > 0
	if isSubQuery {
		b.query.WriteString("(")
	}

	b.buildAndExpression(expr.Left)

	for _, right := range expr.Right {
		b.query.WriteString(" UNION ")
		b.buildAndExpression(right.Right)
	}

	if isSubQuery {
		b.query.WriteString(")")
	}
}

// buildAndExpression handles AND (INTERSECT) and EXCEPT nodes.
func (b *SQLBuilder) buildAndExpression(expr *AndExpression) {
	if expr == nil || (expr.Left == nil && len(expr.Right) == 0) {
		return
	}

	// The logic is a chain of set operations. We must establish the first set
	// in the chain before appending the operations.
	var initialTerm *Term
	var subsequentOps []*OpTerm

	if expr.Left != nil {
		// Case 1: Expression starts with a tag, e.g., "tag1 & tag2"
		initialTerm = expr.Left
		subsequentOps = expr.Right
	} else {
		// Case 2: Expression starts with an operator, e.g., "-tag1 & tag2"
		// The first "set" is now the universal set.
		b.query.WriteString(`(SELECT hash FROM contents)`)
		// All of expr.Right are subsequent operations on this universal set.
		subsequentOps = expr.Right
	}

	// Build the query for the first set if it exists.
	if initialTerm != nil {
		b.buildTerm(initialTerm)
	}

	// Apply all subsequent operations.
	for _, op := range subsequentOps {
		opStr := strings.TrimSpace(op.Operator)
		switch opStr {
		case "-":
			b.query.WriteString(" EXCEPT ")
		default: // & or implicit
			b.query.WriteString(" INTERSECT ")
		}
		b.buildTerm(op.Right)
	}
}

// buildTerm handles individual tags or sub-expressions.
func (b *SQLBuilder) buildTerm(term *Term) {
	if term.SubExpr != nil {
		// Explicitly wrap sub-expressions in parentheses to enforce
		// the user's intended precedence from the query string.
		b.query.WriteString("(")
		b.buildExpression(term.SubExpr)
		b.query.WriteString(")")
	} else if term.Tag != nil {
		b.buildTagQuery(*term.Tag)
	}
}

// buildTagQuery generates the base SELECT statement for a single tag.
func (b *SQLBuilder) buildTagQuery(tagStr string) {
	parsed := ParseTag(tagStr)
	b.query.WriteString(
		`SELECT ct.content_hash as hash FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE t.key = ? AND t.value = ?`,
	)
	b.args = append(b.args, parsed.Key, parsed.Value)
}