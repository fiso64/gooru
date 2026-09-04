from pathlib import Path

p = Path('.maintainer-254-source-modtime.py')
text = p.read_text()
old = '''replace(
    "docs/CONFIG.md",
    "- `uploads.max_file_size_bytes`: maximum bytes accepted per uploaded file. `0` disables the per-file limit.\\n- `uploads.conflict_policy`:",
    "- `uploads.max_file_size_bytes`: maximum bytes accepted per uploaded file. `0` disables the per-file limit.\\n- `uploads.preserve_modtime`: preserve each browser-uploaded file's source modification timestamp on the stored destination. Defaults to `true`; source timestamps are still carried through upload processing when disabled.\\n- `uploads.conflict_policy`:",
)
'''
new = '''replace(
    "docs/CONFIG.md",
    "| `uploads.max_file_size_bytes` | `0` | Optional upload per-file size setting. A zero value leaves the upload-specific size limit unset; set this explicitly when deployments need a hard upload cap. The generic `server.max_request_body_bytes` limit does not cap `/uploads`. |\\n| `uploads.conflict_policy` | `rename` | Default same-name behavior: `skip`, `rename`, `replace`, or `error`. |",
    "| `uploads.max_file_size_bytes` | `0` | Optional upload per-file size setting. A zero value leaves the upload-specific size limit unset; set this explicitly when deployments need a hard upload cap. The generic `server.max_request_body_bytes` limit does not cap `/uploads`. |\\n| `uploads.preserve_modtime` | `true` | Preserve each browser-uploaded file's source modification timestamp on the stored destination. Source timestamps are still carried through upload processing when disabled. |\\n| `uploads.conflict_policy` | `rename` | Default same-name behavior: `skip`, `rename`, `replace`, or `error`. |",
)
replace(
    "docs/CONFIG.md",
    "  max_file_size_bytes: 104857600\\n  conflict_policy: rename",
    "  max_file_size_bytes: 104857600\\n  preserve_modtime: true\\n  conflict_policy: rename",
    2,
)
'''
if text.count(old) != 1:
    raise SystemExit('doc patch source pattern mismatch')
p.write_text(text.replace(old, new))
