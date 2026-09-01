package query

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"gooru.local/types"
)

var virtualMediaKinds = map[string]struct{}{
	"audio": {},
	"gif":   {},
	"other": {},
	"photo": {},
	"video": {},
}

// SQLBuilder transforms a query AST into a SQL query string and arguments.
type SQLBuilder struct {
	query        strings.Builder
	args         []interface{}
	aliasCounter int
	tagCounts    map[string]int
	target       string
}

func (b *SQLBuilder) newAlias() string {
	alias := fmt.Sprintf("t%d", b.aliasCounter)
	b.aliasCounter++
	return alias
}

func Build(expr *Expression, tagCounts map[string]int) (string, []interface{}) {
	if expr == nil || len(expr.Or) == 0 {
		return "", nil
	}
	b := &SQLBuilder{tagCounts: tagCounts, target: "hash"}
	b.buildExpression(expr)
	return b.query.String(), b.args
}

func BuildLocations(expr *Expression, tagCounts map[string]int) (string, []interface{}) {
	if expr == nil || len(expr.Or) == 0 {
		return "", nil
	}
	b := &SQLBuilder{tagCounts: tagCounts, target: "id"}
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

func (b *SQLBuilder) buildAndTerm(andTerm *AndTerm) {
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

	getTermSelectivity := func(term *Term) int {
		if term.Factor == nil || term.Factor.Tag == nil {
			return math.MaxInt32
		}
		if count, ok := b.tagCounts[*term.Factor.Tag]; ok {
			return count
		}
		return math.MaxInt32
	}
	sort.Slice(positiveTerms, func(i, j int) bool {
		return getTermSelectivity(positiveTerms[i]) < getTermSelectivity(positiveTerms[j])
	})

	if len(positiveTerms) == 0 {
		b.query.WriteString(b.allLocationsQuery() + " WHERE 1=1")
		for _, term := range negativeTerms {
			b.query.WriteString(" AND " + b.locationColumn() + " NOT IN (")
			b.buildFactor(term.Factor)
			b.query.WriteString(")")
		}
		return
	}

	b.query.WriteString("SELECT " + b.column() + " FROM (")
	b.buildFactor(positiveTerms[0].Factor)
	b.query.WriteString(fmt.Sprintf(") AS %s WHERE 1=1", b.newAlias()))
	for i := 1; i < len(positiveTerms); i++ {
		b.query.WriteString(" AND " + b.column() + " IN (")
		b.buildFactor(positiveTerms[i].Factor)
		b.query.WriteString(")")
	}
	for _, term := range negativeTerms {
		b.query.WriteString(" AND " + b.column() + " NOT IN (")
		b.buildFactor(term.Factor)
		b.query.WriteString(")")
	}
}

func (b *SQLBuilder) buildTerm(term *Term) {
	if term.Not {
		b.query.WriteString(b.allLocationsQuery() + " WHERE " + b.locationColumn() + " NOT IN (")
		b.buildFactor(term.Factor)
		b.query.WriteString(")")
	} else {
		b.buildFactor(term.Factor)
	}
}

func (b *SQLBuilder) buildFactor(factor *Factor) {
	if factor.SubExpr != nil {
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

func isVirtualMediaKind(value string) bool {
	_, ok := virtualMediaKinds[strings.ToLower(value)]
	return ok
}

func (b *SQLBuilder) buildMediaKindQuery(value string) {
	selectColumn := `DISTINCT l.content_hash as hash`
	if b.target == "id" {
		selectColumn = `l.id as id`
	}
	b.query.WriteString(`SELECT ` + selectColumn + ` FROM locations l LEFT JOIN media_metadata mm ON mm.location_id = l.id WHERE lower(` + mediaKindExpression() + `) = lower(?)`)
	b.args = append(b.args, value)
}

func (b *SQLBuilder) buildUserTagQuery(tagStr string, parsed types.ParsedTag) {
	if parsed.Value == "" && !strings.HasSuffix(tagStr, ":") {
		if b.target == "id" {
			b.query.WriteString(`SELECT DISTINCT l.id as id FROM locations l LEFT JOIN content_tags ct ON l.content_hash = ct.content_hash LEFT JOIN tags t ON ct.tag_id = t.id WHERE (t.key = ? OR lower(l.path) LIKE lower(?))`)
		} else {
			b.query.WriteString(`SELECT DISTINCT l.content_hash as hash FROM locations l LEFT JOIN content_tags ct ON l.content_hash = ct.content_hash LEFT JOIN tags t ON ct.tag_id = t.id WHERE (t.key = ? OR lower(l.path) LIKE lower(?))`)
		}
		b.args = append(b.args, parsed.Key, "%"+parsed.Key+"%")
		return
	}

	queryPrefix := `SELECT DISTINCT l.content_hash as hash FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash JOIN tags t ON ct.tag_id = t.id WHERE `
	if b.target == "id" {
		queryPrefix = `SELECT DISTINCT l.id as id FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash JOIN tags t ON ct.tag_id = t.id WHERE `
	}
	b.query.WriteString(queryPrefix)

	if parsed.Value == "*" {
		b.query.WriteString(`t.key = ? AND t.value != ''`)
		b.args = append(b.args, parsed.Key)
	} else if parsed.Value != "" {
		b.query.WriteString(`t.key = ? AND t.value = ?`)
		b.args = append(b.args, parsed.Key, parsed.Value)
	} else {
		b.query.WriteString(`t.key = ? AND t.value = ''`)
		b.args = append(b.args, parsed.Key)
	}
}

func (b *SQLBuilder) buildTagQuery(tagStr string) {
	if strings.HasPrefix(tagStr, "@") {
		switch tagStr {
		case "@tagged":
			if b.target == "id" {
				b.query.WriteString(`SELECT DISTINCT l.id as id FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash`)
			} else {
				b.query.WriteString(`SELECT DISTINCT l.content_hash as hash FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash`)
			}
		default:
			b.query.WriteString(`SELECT NULL as ` + b.column() + ` WHERE 0`)
		}
		return
	}

	parsed := ParseTag(tagStr)
	switch parsed.Key {
	case "ext":
		if b.target == "id" {
			b.query.WriteString(`SELECT id FROM locations WHERE lower(extension) = lower(?)`)
		} else {
			b.query.WriteString(`SELECT DISTINCT content_hash as hash FROM locations WHERE lower(extension) = lower(?)`)
		}
		value := parsed.Value
		if value != "" && !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		b.args = append(b.args, value)
	case "kind":
		// The UI uses kind:<media-kind> as a virtual field. Keep other kind:* values
		// as ordinary user tags for compatibility with existing libraries.
		if isVirtualMediaKind(parsed.Value) {
			b.buildMediaKindQuery(parsed.Value)
			return
		}
		b.buildUserTagQuery(tagStr, parsed)
	default:
		b.buildUserTagQuery(tagStr, parsed)
	}
}
