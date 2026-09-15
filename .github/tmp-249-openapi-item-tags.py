from pathlib import Path

path = Path("docs/openapi.yaml")
text = path.read_text()
old = '''                tags:\n                  type: string\n                  description: Space- or comma-separated initial tags.\n'''
new = '''                tags:\n                  type: string\n                  description: Space- or comma-separated fallback initial tags applied to files that do not provide item_tags.\n                item_tags:\n                  type: array\n                  items:\n                    type: string\n                  description: Per-file initial tag strings aligned with files. Each entry accepts space- or comma-separated tags; an explicit empty entry means no tags for that file and suppresses fallback tags. Omit item_tags entirely to apply fallback tags to every file.\n'''
if text.count(old) != 1:
    raise SystemExit(f"expected one upload tags schema block, found {text.count(old)}")
path.write_text(text.replace(old, new, 1))
