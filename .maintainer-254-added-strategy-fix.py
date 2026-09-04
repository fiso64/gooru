from pathlib import Path

# Preserve an existing location's original added_at on path upsert; explicit strategy applies to new uploaded locations.
p = Path('.maintainer-254-added-strategy.py')
text = p.read_text()
text = text.replace('\\t\\t\\t\\tadded_at=excluded.added_at`",', '\\t\\t\\t\\tadded_at=locations.added_at`",')
# Fix explicit queue override: only a truly blank request falls back to target config.
old = '''func uploadAddedAtStrategy(requested, fallback string) (string, error) {
\trequested = normalizeAddedAtStrategy(requested)
\tif strings.TrimSpace(requested) == "queue" && strings.TrimSpace(fallback) != "" && strings.TrimSpace(requested) == strings.TrimSpace(normalizeAddedAtStrategy("")) {
\t\t// A blank request normalizes to queue; use the target fallback instead.
\t\trequested = normalizeAddedAtStrategy(fallback)
\t}
\tif !validAddedAtStrategy(requested) {
\t\treturn "", errors.New("added_at_strategy must be one of: queue, reverse_queue, modtime")
\t}
\treturn requested, nil
}'''
new = '''func uploadAddedAtStrategy(requested, fallback string) (string, error) {
\trequested = strings.ToLower(strings.TrimSpace(requested))
\tif requested == "" {
\t\trequested = normalizeAddedAtStrategy(fallback)
\t}
\tif !validAddedAtStrategy(requested) {
\t\treturn "", errors.New("added_at_strategy must be one of: queue, reverse_queue, modtime")
\t}
\treturn requested, nil
}'''
if old not in text:
    raise SystemExit('strategy helper source not found')
text = text.replace(old, new)
p.write_text(text)
