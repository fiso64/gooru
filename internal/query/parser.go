package query

import (
	"regexp"
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
		// Meta-tags may carry one colon-delimited value. Values containing spaces
		// remain available through QuotedString (for example
		// "@filename_contains:summer trip"). Regular tag values may use '*'.
		// A tag cannot start with a hyphen to avoid ambiguity with NOT.
		{Name: "Tag", Pattern: `(@[a-zA-Z0-9_]+(?::[a-zA-Z0-9_./\\:*-]+)?)|([a-zA-Z0-9_./\\][a-zA-Z0-9_./\\:*-]*)`},
		{Name: "Operator", Pattern: `[|()&!-]`},
		{Name: "Whitespace", Pattern: `\s+`},
	})

	parser = participle.MustBuild[Expression](
		participle.Lexer(queryLexer),
		participle.Unquote("QuotedString"),
		participle.Elide("Whitespace"),
	)

	// Regex for normalizing user-friendly query syntax to parser-friendly syntax.
	// case-insensitive ' or ' becomes ' | '
	reOr = regexp.MustCompile(`(?i)\s+or\s+`)
	// case-insensitive ' and ' becomes ' '
	reAnd = regexp.MustCompile(`(?i)\s+and\s+`)
	// case-insensitive 'not ' becomes '-'
	reNot = regexp.MustCompile(`(?i)\bnot\s+`)
	// virtual type tag (e.g. type:img)
	reType = regexp.MustCompile(`\btype:(img|vid)\b`)
)

var virtualTypeTags = map[string][]string{
	"img": {"jpg", "jpeg", "png", "gif", "webp", ".ico", "heic", "heif", "bmp", "tiff", "raw", "cr2", "nef", "arw", "orf"},
	"vid": {"mp4", "mov", "avi", "mkv", "webm", "flv", "wmv", "mpeg", "mpg"},
}

// expandVirtualTypeTags replaces `type:img` or `type:vid` with an OR-group of corresponding extensions.
func expandVirtualTypeTags(expression string) string {
	return reType.ReplaceAllStringFunc(expression, func(match string) string {
		parts := strings.SplitN(match, ":", 2)
		if len(parts) < 2 {
			return match
		}
		typeName := parts[1]
		if extensions, ok := virtualTypeTags[typeName]; ok {
			var expanded []string
			for _, ext := range extensions {
				expanded = append(expanded, "ext:"+ext)
			}
			return "(" + strings.Join(expanded, " | ") + ")"
		}
		return match
	})
}

func normalizeUnquotedSyntax(expression string) string {
	s := expandVirtualTypeTags(expression)
	s = reOr.ReplaceAllString(s, " | ")
	s = reAnd.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&", " ")
	s = reNot.ReplaceAllString(s, "-")
	s = strings.ReplaceAll(s, "!", "-")
	return s
}

// normalizeQuerySyntax applies user-friendly shorthand only outside quoted
// strings. Quoted tags are literals, so words such as "or" and characters such
// as '!' must reach the parser unchanged.
func normalizeQuerySyntax(expression string) string {
	var out strings.Builder
	segmentStart := 0
	inQuote := false

	for i := 0; i < len(expression); i++ {
		if expression[i] != '"' {
			continue
		}

		escaped := false
		for j := i - 1; j >= 0 && expression[j] == '\\'; j-- {
			escaped = !escaped
		}
		if escaped {
			continue
		}

		if !inQuote {
			out.WriteString(normalizeUnquotedSyntax(expression[segmentStart:i]))
			segmentStart = i
			inQuote = true
			continue
		}

		out.WriteString(expression[segmentStart : i+1])
		segmentStart = i + 1
		inQuote = false
	}

	if inQuote {
		// Preserve an unterminated quoted segment so the parser can report the
		// original syntax error rather than normalizing its literal contents.
		out.WriteString(expression[segmentStart:])
	} else {
		out.WriteString(normalizeUnquotedSyntax(expression[segmentStart:]))
	}
	return out.String()
}

// Parse takes a query expression string and returns the parsed AST.
func Parse(expression string) (*Expression, error) {
	return parser.ParseString("", normalizeQuerySyntax(expression))
}
