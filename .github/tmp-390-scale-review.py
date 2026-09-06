from pathlib import Path


def replace(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"expected snippet missing in {path}: {old[:80]!r}")
    p.write_text(text.replace(old, new, 1))

replace(
    "internal/database/database.go",
    """\t\tWITH matching_keys AS (\n\t\t\tSELECT t.key AS tag_str, COUNT(DISTINCT ct.content_hash) AS files_count\n\t\t\tFROM tags t\n\t\t\tJOIN content_tags ct ON ct.tag_id = t.id\n\t\t\tWHERE lower(t.key) LIKE ?\n\t\t\tGROUP BY t.key\n\t\t), matching_values AS (""",
    """\t\tWITH matching_keys AS (\n\t\t\tSELECT key AS tag_str, SUM(files_count) AS files_count\n\t\t\tFROM tags\n\t\t\tWHERE lower(key) LIKE ?\n\t\t\tGROUP BY key\n\t\t), matching_values AS (""",
)
replace(
    "internal/database/database.go",
    """\t\tSELECT t.key || ':' AS tag_str, COUNT(DISTINCT ct.content_hash) AS total_files\n\t\tFROM tags t\n\t\tJOIN content_tags ct ON ct.tag_id = t.id\n\t\tWHERE t.value != '' AND lower(t.key) LIKE ?\n\t\tGROUP BY t.key""",
    """\t\tSELECT key || ':' AS tag_str, SUM(files_count) AS total_files\n\t\tFROM tags\n\t\tWHERE value != '' AND lower(key) LIKE ?\n\t\tGROUP BY key""",
)

replace(
    "frontend/src/lib/utils/tagSuggestions.ts",
    """  // The base key count is the unique-file aggregate for both the plain key and\n  // its namespace completion. Promote derived namespace counts to that same\n  // aggregate when the base candidate is present; a plain-only key still never\n  // creates a namespace candidate.\n  for (const [namespaceName, item] of seenNamespaces) {\n    const base = seenTags.get(namespaceName.slice(0, -1));\n    if (base) seenNamespaces.set(namespaceName, { ...item, count: base.count });\n  }\n""",
    """  // Prefer an explicit namespace aggregate from the API. When the initial cached\n  // candidate set only has concrete key:value tags, derive the namespace count\n  // by summing those usage counts. Plain-only keys never create namespaces.\n  const explicitNamespaceCounts = new Map<string, number>();\n  const derivedNamespaceCounts = new Map<string, number>();\n  for (const candidate of candidates) {\n    const name = candidateTagName(candidate).trim();\n    const separator = name.indexOf(':');\n    const namespace = candidate.namespace?.trim() || (separator > 0 ? name.slice(0, separator) : '');\n    if (!namespace) continue;\n    const namespaceName = `${namespace}:`;\n    if (name === namespaceName) {\n      explicitNamespaceCounts.set(namespaceName, candidate.count ?? 0);\n    } else {\n      derivedNamespaceCounts.set(namespaceName, (derivedNamespaceCounts.get(namespaceName) ?? 0) + (candidate.count ?? 0));\n    }\n  }\n  for (const [namespaceName, item] of seenNamespaces) {\n    const count = explicitNamespaceCounts.get(namespaceName) ?? derivedNamespaceCounts.get(namespaceName) ?? item.count;\n    seenNamespaces.set(namespaceName, { ...item, count });\n  }\n""",
)

p = Path("internal/database/database_test.go")
text = p.read_text()
text = text.replace("TestTagSuggestionsUseUniqueFileAggregatesForBaseAndNamespace", "TestTagSuggestionsUseMaintainedUsageAggregatesForBaseAndNamespace", 1)
if 'counts["animal"] != 3' not in text:
    raise SystemExit("animal assertion missing")
text = text.replace('counts["animal"] != 3', 'counts["animal"] != 5', 1)
text = text.replace('animal count=%d want 3', 'animal count=%d want 5', 1)
if 'namespaces[0].Count != 3' not in text:
    raise SystemExit("namespace assertion missing")
text = text.replace('namespaces[0].Count != 3', 'namespaces[0].Count != 4', 1)
text = text.replace('want animal: count 3', 'want animal: count 4', 1)
p.write_text(text)
