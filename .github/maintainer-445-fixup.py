from pathlib import Path
p = Path('.github/maintainer-445-apply.py')
s = p.read_text()
s = s.replace("replace('internal/serve/config_upload_policy_test.go', 'default upload conflict policy = %q, want skip', 'default upload conflict policy = %q, want rename')", "replace('internal/serve/config_upload_policy_test.go', 'expected default uploads.conflict_policy=skip, got %q', 'expected default uploads.conflict_policy=rename, got %q')")
s = s.replace("replace('internal/serve/config_upload_policy_test.go', 'empty upload conflict policy = %q, want skip', 'empty upload conflict policy = %q, want rename')", "replace('internal/serve/config_upload_policy_test.go', 'expected omitted conflict policy to resolve to skip, got %q', 'expected omitted conflict policy to resolve to rename, got %q')")
p.write_text(s)
