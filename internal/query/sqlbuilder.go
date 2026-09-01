package query

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// SQLBuilder transforms a query AST into a SQL query string and arguments.
type SQLBuilder struct {
	query        strings.Builder
	args         []interface{}
	aliasCounter int
	tagCounts    map[string]int
	target       string
}

// newAlias creates a unique alias for a derived table (e.g., t0, t1, ...).
func (b *SQLBuilder) newAlias() string {
	alias := fmt.Sprintf("t%d", b.aliasCounter)
	b.aliasCounter++
	return alias
}

// Build generates the SQL query from the AST.
// It uses tagCounts to intelligently order AND clauses for performance.
func Build(expr *Expression, tagCounts map[string]int) (string, []interface{}) {
	if expr == nil || len(expr.Or) == 0 {
		return "", nil
	}
	b := &SQLBuilder{
		tagCounts: tagCounts,
		target:    "hash",
	}
	b.buildExpression(expr)
	return b.query.String(), b.args
}

// BuildLocations generates a query that returns matching location IDs. Unlike
// Build, filename/path predicates remain location-scoped instead of expanding
// through shared content hashes.
func BuildLocations(expr *Expression, tagCounts map[string]int) (string, []interface{}) {
	if expr == nil || len(expr.Or) == 0 {
		return "", nil
	}
	b := &SQLBuilder{
		tagCounts: tagCounts,
		target:    "id",
	}
	b.buildExpression(expr)
	return b.query.String(), b.args
}

func (b *SQLBuilder) column() string {
	if b.target == "id" {
		return "id"
	}
	return "hash"
}

func (b *SQLBuilder) locationColumn() string {
	if b.target == "id" {
		return "id"
	}
	return "content_hash"
}

func (b *SQLBuilder) allLocationsQuery() string {
	if b.target == "id" {
		return "SELECT id FROM locations"
	}
	return "SELECT DISTINCT content_hash as hash FROM locations"
}

// buildExpression handles OR nodes. If there are multiple OR operands, the entire
// resulting compound query is wrapped in a derived table to make it a valid SELECT statement.
func (b *SQLBuilder) buildExpression(expr *Expression) {
	isCompound := len(expr.Or) > 1
	if isCompound {
		b.query.WriteString("SELECT " + b.column() + " FROM (")
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

// buildAndTerm handles AND nodes. It intelligently sorts positive terms by
// their file counts (selectivity) to ensure the most restrictive filter is
// applied first, leading to significant performance gains.
func (b *SQLBuilder) buildAndTerm(andTerm *AndTerm) {
	// If there's only one term, no complex AND logic is needed.
	if len(andTerm.And) == 1 {
		b.buildTerm(andTerm.And[0])
		return
	}

	var positiveTerms []*Term
	var negativeTerms []*Term

	for _, term := range andTerm.And {
		if term.Not {
			negativeTerms = append(negativeTerms, term)
		} else {
			positiveTerms = append(positiveTerms, term)
		}
	}

	// getTermSelectivity is a helper to safely determine the count/cost of a term.
	getTermSelectivity := func(term *Term) int {
		// If the factor is a sub-expression, its tag is nil. Treat it as least selective.
		if term.Factor == nil || term.Factor.Tag == nil {
			return math.MaxInt32
		}
		tagStr := *term.Factor.Tag
		// If the tag's count is known, use it. Otherwise (e.g. for a virtual tag),
		// treat it as least selective to prioritize known, counted tags.
		if count, ok := b.tagCounts[tagStr]; ok {
			return count
		}
		return math.MaxInt32
	}

	// Sort positive terms by their selectivity (file count, ascending).
	sort.Slice(positiveTerms, func(i, j int) bool {
		// Use the safe helper function to get the selectivity.
		return getTermSelectivity(positiveTerms[i]) < getTermSelectivity(positiveTerms[j])
	})

	if len(positiveTerms) == 0 {
		// Case: The query is composed entirely of negative terms (e.g., "-a -b").
		// We start with the set of all content that has a location and filter it down.
		b.query.WriteString(b.allLocationsQuery() + " WHERE 1=1")
		for _, term := range negativeTerms {
			b.query.WriteString(" AND " + b.locationColumn() + " NOT IN (")
			b.buildFactor(term.Factor)
			b.query.WriteString(")")
		}
		return
	}

	// Case: The query has at least one positive term (e.g., "a & b & -c").
	// We use the first positive term (which is now the most selective) as the base set.
	b.query.WriteString("SELECT " + b.column() + " FROM (")
	b.buildFactor(positiveTerms[0].Factor)
	b.query.WriteString(fmt.Sprintf(") AS %s WHERE 1=1", b.newAlias()))

	// Filter this base set by requiring matches in all other positive terms.
	for i := 1; i < len(positiveTerms); i++ {
		b.query.WriteString(" AND " + b.column() + " IN (")
		b.buildFactor(positiveTerms[i].Factor)
		b.query.WriteString(")")
	}

	// Further filter the set by excluding matches from all negative terms.
	for _, term := range negativeTerms {
		b.query.WriteString(" AND " + b.column() + " NOT IN (")
		b.buildFactor(term.Factor)
		b.query.WriteString(")")
	}
}

// buildTerm handles NOT nodes for simple cases (e.g., a single negated term).
// The logic for complex ANDs with negations is handled in buildAndTerm.
func (b *SQLBuilder) buildTerm(term *Term) {
	if term.Not {
		// The base set for a negation must be files that actually exist (have a location).
		b.query.WriteString(b.allLocationsQuery() + " WHERE " + b.locationColumn() + " NOT IN (")
		b.buildFactor(term.Factor)
		b.query.WriteString(")")
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

func mediaKindExpression() string {
	return `coalesce(mm.media_kind, CASE
		WHEN lower(l.extension) = '.gif' THEN 'gif'
		WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
		WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
		ELSE 'other'
	END)`
}

// buildTagQuery generates the base, simple SELECT statement for a single tag,
// handling both normal tags and virtual metadata tags.
func (b *SQLBuilder) buildTagQuery(tagStr string) {
	// Handle meta-tags first, as they don't follow the key:value structure.
	if strings.HasPrefix(tagStr, "@") {
		switch tagStr {
		case "@tagged":
			// Query for all content that is both tagged AND has a location.
			if b.target == "id" {
				b.query.WriteString(`SELECT DISTINCT l.id as id FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash`)
			} else {
				b.query.WriteString(`SELECT DISTINCT l.content_hash as hash FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash`)
			}
		// In the future, other meta-tags like @orphaned could be added here.
		default:
			// For now, treat unknown meta-tags as "no results".
			b.query.WriteString(`SELECT NULL as ` + b.column() + ` WHERE 0`)
		}
		return
	}

	parsed := ParseTag(tagStr)

	switch parsed.Key {
	case "ext":
		// Query against the indexed, lowercase extension in the locations table.
		if b.target == "id" {
			b.query.WriteString(`SELECT id FROM locations WHERE lower(extension) = lower(?)`)
		} else {
			b.query.WriteString(`SELECT DISTINCT content_hash as hash FROM locations WHERE lower(extension) = lower(?)`)
		}
		value := parsed.Value
		// Add leading dot to extension if missing, for user convenience.
		if value != "" && !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		b.args = append(b.args, value)
	case "kind":
		// Media kind is a virtual query field backed by extracted metadata, with the
		// same extension fallback used by kind facets for files not yet analyzed.
		selectColumn := `DISTINCT l.content_hash as hash`
		if b.target == "id" {
			selectColumn = `l.id as id`
		}
		b.query.WriteString(`SELECT ` + selectColumn + ` FROM locations l LEFT JOIN media_metadata mm ON mm.location_id = l.id WHERE lower(` + mediaKindExpression() + `) = lower(?)`)
		b.args = append(b.args, parsed.Value)
	// Add other virtual tags like 'size', 'path', etc. here in the future.
	default:
		if parsed.Value == "" && !strings.HasSuffix(tagStr, ":") {
			// A bare term acts as both a key-only tag search and filename/path
			// free-text search for browser search boxes. Use LEFT JOIN so
			// filename matches include untagged tracked files.
			if b.target == "id" {
				b.query.WriteString(`SELECT DISTINCT l.id as id FROM locations l LEFT JOIN content_tags ct ON l.content_hash = ct.content_hash LEFT JOIN tags t ON ct.tag_id = t.id WHERE (t.key = ? OR lower(l.path) LIKE lower(?))`)
			} else {
				b.query.WriteString(`SELECT DISTINCT l.content_hash as hash FROM locations l LEFT JOIN content_tags ct ON l.content_hash = ct.content_hash LEFT JOIN tags t ON ct.tag_id = t.id WHERE (t.key = ? OR lower(l.path) LIKE lower(?))`)
			}
			b.args = append(b.args, parsed.Key, "%"+parsed.Key+"%")
			return
		}
		// Default behavior for user-defined tags.
		// The common prefix ensures we only consider content that has a location.
		queryPrefix := `SELECT DISTINCT l.content_hash as hash FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash JOIN tags t ON ct.tag_id = t.id WHERE `
		if b.target == "id" {
			queryPrefix = `SELECT DISTINCT l.id as id FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash JOIN tags t ON ct.tag_id = t.id WHERE `
		}
		b.query.WriteString(queryPrefix)

		if parsed.Value == "*" {
			// Query for a key with any non-empty value (e.g., "location:*").
			b.query.WriteString(`t.key = ? AND t.value != ''`)
			b.args = append(b.args, parsed.Key)
		} else if parsed.Value != "" {
			// Query for a specific key:value pair (e.g., "location:home").
			b.query.WriteString(`t.key = ? AND t.value = ?`)
			b.args = append(b.args, parsed.Key, parsed.Value)
		} else {
			// The user explicitly typed the colon, so they want an empty value (e.g., "location:").
			b.query.WriteString(`t.key = ? AND t.value = ''`)
			b.args = append(b.args, parsed.Key)
		}
	}
}
