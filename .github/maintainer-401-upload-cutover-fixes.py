from pathlib import Path


# The synchronous importer-only fallback should still avoid staging work for an
# already-canceled request even though it no longer reserves a legacy job.
p = Path("internal/serve/uploads.go")
text = p.read_text()
marker = '''\t// Real GooruLibrary uploads are selected into handleDurableUpload before
\t// reaching this fallback. Importer-only doubles and embedders cannot safely
'''
insert = '''\tif err := r.Context().Err(); err != nil {
\t\twriteError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
\t\treturn
\t}
'''
if marker not in text:
    raise SystemExit("uploads.go: fallback marker missing")
text = text.replace(marker, insert + marker, 1)
p.write_text(text)


# These tests assert the removed in-memory upload JobManager path. Durable
# equivalents cover admission-before-staging, async operation identity, bounded
# queue rejection, and queued cancellation cleanup.
p = Path("internal/serve/uploads_test.go")
text = p.read_text()

def remove_function(source: str, name: str) -> str:
    needle = f"func {name}("
    start = source.find(needle)
    if start < 0:
        raise SystemExit(f"uploads_test.go: {name} not found")
    brace = source.find("{", start)
    if brace < 0:
        raise SystemExit(f"uploads_test.go: {name} opening brace missing")
    depth = 0
    i = brace
    while i < len(source):
        if source[i] == "{":
            depth += 1
        elif source[i] == "}":
            depth -= 1
            if depth == 0:
                end = i + 1
                while end < len(source) and source[end] == "\n":
                    end += 1
                return source[:start] + source[end:]
        i += 1
    raise SystemExit(f"uploads_test.go: {name} closing brace missing")

for name in (
    "TestUploadAdmissionRejectsBeforeParsingBody",
    "TestUploadAsyncReturnsJob",
    "TestUploadQueueFullReturnsStableJSONError",
    "TestCanceledQueuedUploadCleansStagedFiles",
):
    text = remove_function(text, name)
p.write_text(text)
