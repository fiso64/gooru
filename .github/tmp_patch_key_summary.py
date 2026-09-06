from pathlib import Path

p = Path("internal/database/database.go")
s = p.read_text()

old = "\trows, err := s.Query(`SELECT DISTINCT key FROM tags WHERE value != '' ORDER BY key`)"
new = """\trows, err := s.Query(`
\t\tSELECT tk.key
\t\tFROM tag_key_counts tk
\t\tWHERE EXISTS (
\t\t\tSELECT 1
\t\t\tFROM tags t
\t\t\tWHERE t.key = tk.key AND t.value != ''
\t\t)
\t\tORDER BY tk.key
\t`)"""
if old not in s:
    raise SystemExit("ListTagNamespaces query not found")
s = s.replace(old, new, 1)

start = s.index("// GetCountForKey gets the count of distinct files for all tags with a given key.")
end = s.index("\n}\n\n// ExistsForKey", start) + 2
replacement = """// GetCountForKey gets the maintained distinct-file count for a tag key.
func (s *Store) GetCountForKey(key string) (int, error) {
\tvar count int
\terr := s.QueryRow(\"SELECT files_count FROM tag_key_counts WHERE key = ?\", key).Scan(&count)
\tif err == sql.ErrNoRows {
\t\treturn 0, nil
\t}
\treturn count, err
}"""
s = s[:start] + replacement + s[end:]
p.write_text(s)
