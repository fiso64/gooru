from pathlib import Path
import re

p = Path("internal/query/tag.go")
s = p.read_text()
if "func ParseTags(" in s:
    raise SystemExit("ParseTags already exists")
s = s.rstrip() + """

// ParseTags parses a collection of tag strings while preserving caller order.
func ParseTags(tags []string) []types.ParsedTag {
	parsedTags := make([]types.ParsedTag, len(tags))
	for i, tag := range tags {
		parsedTags[i] = ParseTag(tag)
	}
	return parsedTags
}
"""
p.write_text(s + "\n")

p = Path("gooru/tagging.go")
s = p.read_text()
old = '''import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
'''
new = '''import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
'''
if s.count(old) != 1:
    raise SystemExit("unexpected import block")
s = s.replace(old, new)

parse = re.compile(
    r"(?m)^(?P<i>[ \t]*)parsedTags := make\(\[\]types\.ParsedTag, len\(tags\)\)\n"
    r"(?P=i)for i, t := range tags \{\n"
    r"(?P=i)\tparsedTags\[i\] = query\.ParseTag\(t\)\n"
    r"(?P=i)\}"
)
s, n = parse.subn(lambda m: m.group("i") + "parsedTags := query.ParseTags(tags)", s)
if n != 5:
    raise SystemExit(f"expected 5 parse loops, got {n}")

ids = re.compile(
    r"(?m)^(?P<i>[ \t]*)tagIDs := make\(\[\]int64, 0, len\(tagIDMap\)\)\n"
    r"(?P=i)for _, id := range tagIDMap \{\n"
    r"(?P=i)\ttagIDs = append\(tagIDs, id\)\n"
    r"(?P=i)\}"
)
s, n = ids.subn(lambda m: m.group("i") + "tagIDs := slices.Collect(maps.Values(tagIDMap))", s)
if n != 3:
    raise SystemExit(f"expected 3 unordered ID loops, got {n}")

p.write_text(s)
