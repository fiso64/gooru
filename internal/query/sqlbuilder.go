package query

import (
	"fmt"
	"strings"
)

// SQLBuilder transforms a query AST into a SQL query string and arguments.
type SQLBuilder struct {
	query        strings.Builder
	args         []interface{}
	aliasCounter int
}

// newAlias creates a unique alias for a derived table (e.g., t0, t1, ...).
func (b *SQLBuilder) newAlias() string {
	alias := fmt.Sprintf("t%d", b.aliasCounter)
	b.aliasCounter++
	return alias
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

// buildExpression handles OR nodes. If there are multiple OR operands, the entire
// resulting compound query is wrapped in a derived table to make it a valid SELECT statement.
func (b *SQLBuilder) buildExpression(expr *Expression) {
	isCompound := len(expr.Or) > 1
	if isCompound {
		b.query.WriteString("SELECT hash FROM (")
	}

	for i, andTerm := range expr.Or {
		if i > 0 {
			b.query.WriteString(" UNION ")
		}
		b.buildAndTerm(andTerm)
	}

	if isCompound {
		b.query.WriteString(fmt.Sprintf(") AS %s", b.newAlias()))
	}
}

// buildAndTerm handles AND nodes. If there are multiple AND operands, it wraps the result.
func (b *SQLBuilder) buildAndTerm(andTerm *AndTerm) {
	isCompound := len(andTerm.And) > 1
	if isCompound {
		b.query.WriteString("SELECT hash FROM (")
	}

	for i, term := range andTerm.And {
		if i > 0 {
			b.query.WriteString(" INTERSECT ")
		}
		b.buildTerm(term)
	}

	if isCompound {
		b.query.WriteString(fmt.Sprintf(") AS %s", b.newAlias()))
	}
}

// buildTerm handles NOT nodes. A negated term is inherently a compound query (U - A)
// and must be wrapped to be a valid SELECT statement.
func (b *SQLBuilder) buildTerm(term *Term) {
	if term.Not {
		b.query.WriteString("SELECT hash FROM (SELECT hash FROM contents EXCEPT ")
		b.buildFactor(term.Factor)
		b.query.WriteString(fmt.Sprintf(") AS %s", b.newAlias()))
	} else {
		b.buildFactor(term.Factor)
	}
}

// buildFactor handles the base units: a tag or a user-defined sub-expression.
func (b *SQLBuilder) buildFactor(factor *Factor) {
	if factor.SubExpr != nil {
		// A user-defined sub-expression is evaluated recursively. The resulting SQL
		// is already a valid SELECT statement due to the logic in the other build functions.
		b.buildExpression(factor.SubExpr)
	} else if factor.Tag != nil {
		b.buildTagQuery(*factor.Tag)
	}
}

// buildTagQuery generates the base, simple SELECT statement for a single tag,
// handling both normal tags and virtual metadata tags.
func (b *SQLBuilder) buildTagQuery(tagStr string) {
	// Handle meta-tags first, as they don't follow the key:value structure.
	if strings.HasPrefix(tagStr, "@") {
		switch tagStr {
		case "@tagged":
			b.query.WriteString(`SELECT DISTINCT content_hash as hash FROM content_tags`)
		// In the future, other meta-tags like @orphaned could be added here.
		default:
			// For now, treat unknown meta-tags as "no results".
			b.query.WriteString(`SELECT NULL as hash WHERE 0`)
		}
		return
	}

	parsed := ParseTag(tagStr)

	switch parsed.Key {
	case "ext":
		// Query against the indexed, lowercase extension in the locations table.
		b.query.WriteString(`SELECT content_hash as hash FROM locations WHERE lower(extension) = lower(?)`)
		value := parsed.Value
		// Add leading dot to extension if missing, for user convenience.
		if value != "" && !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		b.args = append(b.args, value)
	// Add other virtual tags like 'size', 'path', etc. here in the future.
	default:
		// Default behavior for user-defined tags
		if parsed.Value == "*" {
			// Query for a key with any non-empty value (e.g., "location:*").
			b.query.WriteString(`SELECT ct.content_hash as hash FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE t.key = ? AND t.value != ''`)
			b.args = append(b.args, parsed.Key)
		} else if parsed.Value != "" {
			// Query for a specific key:value pair (e.g., "location:home").
			b.query.WriteString(`SELECT ct.content_hash as hash FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE t.key = ? AND t.value = ?`)
			b.args = append(b.args, parsed.Key, parsed.Value)
		} else {
			// This is the ambiguous case: `location` vs `location:`
			if strings.HasSuffix(tagStr, ":") {
				// The user explicitly typed the colon, so they want an empty value (e.g., "location:").
				b.query.WriteString(`SELECT ct.content_hash as hash FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE t.key = ? AND t.value = ''`)
				b.args = append(b.args, parsed.Key)
			} else {
				// Query for a key, regardless of value (e.g., "location").
				b.query.WriteString(`SELECT ct.content_hash as hash FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE t.key = ?`)
				b.args = append(b.args, parsed.Key)
			}
		}
	}
}
