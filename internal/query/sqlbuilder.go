package query

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

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
		tagStr := *term.Factor.Tag
		if count, ok := b.tagCounts[tagStr]; ok {
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

func mediaTypeExpression() string {
	return `CASE
		WHEN lower(l.extension) = '.cbz' THEN 'comic'
		ELSE coalesce(mm.media_kind, CASE
			WHEN lower(l.extension) = '.gif' THEN 'gif'
			WHEN lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif') THEN 'photo'
			WHEN lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg') THEN 'video'
			ELSE 'other'
		END)
	END`
}

func fallbackMediaTypeCondition(value string) string {
	switch strings.ToLower(value) {
	case "comic":
		return `lower(l.extension) = '.cbz'`
	case "gif":
		return `lower(l.extension) = '.gif'`
	case "photo":
		return `lower(l.extension) IN ('.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif')`
	case "video":
		return `lower(l.extension) IN ('.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg')`
	case "other":
		return `lower(l.extension) NOT IN ('.cbz', '.gif', '.jpg', '.jpeg', '.png', '.webp', '.bmp', '.tif', '.tiff', '.heic', '.heif', '.mp4', '.mov', '.avi', '.mkv', '.webm', '.flv', '.wmv', '.mpeg', '.mpg')`
	default:
		return ""
	}
}

func (b *SQLBuilder) buildMediaTypeQuery(value string) {
	// Metadata-backed kinds and extension-derived fallback kinds are mutually
	// exclusive for a location. Split them so SQLite can use the media-kind and
	// extension indexes instead of evaluating a COALESCE/CASE over every row.
	selectColumn := `l.content_hash as hash`
	union := ` UNION `
	if b.target == "id" {
		selectColumn = `l.id as id`
		union = ` UNION ALL `
	}

	b.query.WriteString(`SELECT ` + selectColumn + ` FROM media_metadata mm JOIN locations l ON l.content_hash = mm.content_hash WHERE lower(l.extension) <> '.cbz' AND lower(mm.media_kind) = lower(?)`)
	b.args = append(b.args, value)

	if strings.EqualFold(value, "comic") {
		b.query.WriteString(union + `SELECT ` + selectColumn + ` FROM locations l WHERE lower(l.extension) = '.cbz'`)
		return
	}

	fallback := fallbackMediaTypeCondition(value)
	if fallback == "" {
		return
	}
	b.query.WriteString(union + `SELECT ` + selectColumn + ` FROM locations l WHERE ` + fallback + ` AND NOT EXISTS (SELECT 1 FROM media_metadata mm WHERE mm.content_hash = l.content_hash)`)
}

func quoteFTS5Phrase(value string) string {
	return "\"" + strings.ReplaceAll(value, "\"", "\"\"") + "\""
}

func (b *SQLBuilder) buildFilenameContainsQuery(value string) {
	selectColumn := `DISTINCT l.content_hash as hash`
	if b.target == "id" {
		selectColumn = `l.id as id`
	}

	b.query.WriteString(`SELECT ` + selectColumn + ` FROM location_filenames lf JOIN locations l ON l.id = lf.rowid WHERE `)
	if utf8.RuneCountInString(value) >= 3 {
		// Use the trigram FTS index to narrow candidates, then retain an exact
		// literal substring guard so punctuation keeps filename_contains semantics.
		b.query.WriteString(`lf.filename MATCH ? AND `)
		b.args = append(b.args, quoteFTS5Phrase(value))
	}
	// Trigram tokenization cannot accelerate terms shorter than three runes. In
	// that case scan the compact basename index, not recursively split full paths.
	b.query.WriteString(`instr(lower(lf.filename), lower(?)) > 0`)
	b.args = append(b.args, value)
}

func (b *SQLBuilder) buildInTargetQuery(value string) {
	selectColumn := `DISTINCT l.content_hash as hash`
	if b.target == "id" {
		selectColumn = `l.id as id`
	}
	b.query.WriteString(`SELECT ` + selectColumn + ` FROM managed_storage_target_locations mstl JOIN locations l ON l.id = mstl.location_id`)
	if value == "any" {
		return
	}
	b.query.WriteString(` WHERE mstl.target_id = ?`)
	b.args = append(b.args, value)
}

func (b *SQLBuilder) buildExternalQuery() {
	if b.target == "id" {
		b.query.WriteString(`SELECT l.id as id FROM locations l WHERE NOT EXISTS (SELECT 1 FROM managed_storage_target_locations mstl WHERE mstl.location_id = l.id)`)
		return
	}
	// External is a content-level predicate: if any location for this content is
	// in a managed target, the content is not external even when another copy is.
	b.query.WriteString(`SELECT DISTINCT l.content_hash as hash FROM locations l WHERE NOT EXISTS (SELECT 1 FROM locations ml JOIN managed_storage_target_locations mstl ON mstl.location_id = ml.id WHERE ml.content_hash = l.content_hash)`)
}

// buildTagQuery generates the base, simple SELECT statement for a single tag,
// handling both normal tags and virtual metadata tags.
func (b *SQLBuilder) buildTagQuery(tagStr string) {
	if strings.HasPrefix(tagStr, "@") {
		meta, err := ParseMetaTag(tagStr)
		if err != nil {
			b.query.WriteString(`SELECT NULL as ` + b.column() + ` WHERE 0`)
			return
		}
		switch meta.Definition.Name {
		case MetaTagTagged:
			if b.target == "id" {
				b.query.WriteString(`SELECT DISTINCT l.id as id FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash`)
			} else {
				b.query.WriteString(`SELECT DISTINCT l.content_hash as hash FROM locations l JOIN content_tags ct ON l.content_hash = ct.content_hash`)
			}
		case MetaTagFilenameContains:
			b.buildFilenameContainsQuery(meta.Value)
		case MetaTagExternal:
			b.buildExternalQuery()
		case MetaTagInTarget:
			b.buildInTargetQuery(meta.Value)
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
	case "type":
		b.buildMediaTypeQuery(parsed.Value)
	default:
		if parsed.Value != "" && parsed.Value != "*" {
			// Exact tag associations are unique by (content_hash, tag_id). Resolve
			// the tag once, then walk the covering tag_id/content_hash index instead
			// of joining tags and locations only to DISTINCT the result again.
			if b.target == "id" {
				b.query.WriteString(`SELECT l.id as id FROM content_tags ct JOIN locations l ON l.content_hash = ct.content_hash WHERE ct.tag_id = (SELECT id FROM tags WHERE key = ? AND value = ?)`)
			} else {
				b.query.WriteString(`SELECT ct.content_hash as hash FROM content_tags ct WHERE ct.tag_id = (SELECT id FROM tags WHERE key = ? AND value = ?)`)
			}
			b.args = append(b.args, parsed.Key, parsed.Value)
			return
		}

		// Bare key-wide location queries are frequently high-cardinality (for
		// example the UI's `hidden` filter). Keep driving populated keys from
		// locations so the outer browse query can preserve ordering and stop at
		// the page limit. For a key known to be empty, drive from tag associations
		// instead so the exclusion subquery proves emptiness without scanning every
		// location. Cardinality only selects a plan; both queries read live tables
		// and remain semantically equivalent if the summary changes concurrently.
		if b.target == "id" && parsed.Value == "" && !strings.HasSuffix(tagStr, ":") {
			if count, ok := b.tagCounts[tagStr]; ok && count == 0 {
				b.query.WriteString(`SELECT DISTINCT l.id as id FROM tags t JOIN content_tags ct ON ct.tag_id = t.id JOIN locations l ON l.content_hash = ct.content_hash WHERE t.key = ?`)
			} else {
				b.query.WriteString(`SELECT l.id as id FROM locations l WHERE EXISTS (SELECT 1 FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE ct.content_hash = l.content_hash AND t.key = ?)`)
			}
			b.args = append(b.args, parsed.Key)
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
		} else if strings.HasSuffix(tagStr, ":") {
			b.query.WriteString(`t.key = ? AND t.value = ''`)
			b.args = append(b.args, parsed.Key)
		} else {
			b.query.WriteString(`t.key = ?`)
			b.args = append(b.args, parsed.Key)
		}
	}
}
