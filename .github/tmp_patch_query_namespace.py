from pathlib import Path

p = Path("gooru/query.go")
s = p.read_text()

old = "\tparsedTags := query.ToParsedTags(userTags)\n\ttagCounts, err := c.store.BatchGetTagCounts(parsedTags)"
new = "\ttagCounts, err := c.store.BatchGetQueryTagCounts(userTags)"
if old not in s:
    raise SystemExit("buildQuery count block not found")
s = s.replace(old, new, 1)

old = "\tparsedTags := query.ToParsedTags(query.ExtractTags(ast))\n\ttagCounts, err := c.store.BatchGetTagCounts(parsedTags)"
new = "\tuserTags := query.ExtractTags(ast)\n\ttagCounts, err := c.store.BatchGetQueryTagCounts(userTags)"
if old not in s:
    raise SystemExit("buildLocationQuery count block not found")
s = s.replace(old, new, 1)

if s.count("BatchGetQueryTagCounts(userTags)") != 2:
    raise SystemExit("expected exactly two query-aware count call sites")
p.write_text(s)
