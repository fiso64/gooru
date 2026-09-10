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


# The jobs query module still exposes finished legacy-job clearing for the one
# remaining tag-mutation JobManager producer. The primary patch removes the
# import while moving upload polling to durable operations, so restore it until
# the following tag-mutation migration removes that API entirely.
p = Path("frontend/src/lib/queries/jobs.ts")
text = p.read_text()
api_import = "import { ApiClient } from '$lib/api/client';\n"
if api_import not in text:
    text = text.replace("import { createMutation, createQuery } from '@tanstack/svelte-query';\n", "import { createMutation, createQuery } from '@tanstack/svelte-query';\n" + api_import, 1)
p.write_text(text)


# Upload submission now returns a BackgroundOperation, while polling/cancel
# still feeds the workflow through the existing Job-shaped presentation model.
# Test fixtures intentionally satisfy both shapes so each test can exercise the
# same UI state transitions without preserving a fake legacy upload producer.
def make_dual_job_fixture(path: str, old_import: str, old_fixture: str, new_fixture: str) -> None:
    p = Path(path)
    text = p.read_text()
    if old_import not in text:
        raise SystemExit(f"{path}: Job import marker missing")
    text = text.replace(old_import, old_import + "import type { BackgroundOperation } from '$lib/api/operations';\n", 1)
    if old_fixture not in text:
        raise SystemExit(f"{path}: pendingJob fixture marker missing")
    text = text.replace(old_fixture, new_fixture, 1)
    p.write_text(text)

make_dual_job_fixture(
    "frontend/src/lib/state/uploadWorkflow.svelte.test.ts",
    "import type { Job } from '$lib/api/types';\n",
    '''function pendingJob(id: string): Job {
  return {
    id,
    type: 'upload_import',
    status: 'pending',
    submitted_at: '2026-09-01T00:00:00Z'
  } as Job;
}
''',
    '''function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 1,
    progress_completed: 0,
    progress_failed: 0,
    submitted_at: '2026-09-01T00:00:00Z',
    created_at: '2026-09-01T00:00:00Z'
  } as Job & BackgroundOperation;
}
''',
)

make_dual_job_fixture(
    "frontend/src/lib/state/uploadWorkflow.admission.test.ts",
    "import type { Job } from '$lib/api/types';\n",
    '''function pendingJob(id: string): Job {
  return {
    id,
    type: 'upload_import',
    status: 'pending',
    submitted_at: '2026-09-07T00:00:00Z'
  } as Job;
}
''',
    '''function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 1,
    progress_completed: 0,
    progress_failed: 0,
    submitted_at: '2026-09-07T00:00:00Z',
    created_at: '2026-09-07T00:00:00Z'
  } as Job & BackgroundOperation;
}
''',
)

make_dual_job_fixture(
    "frontend/src/lib/state/uploadWorkflow.queueFull.test.ts",
    "import type { Job } from '$lib/api/types';\n",
    '''function pendingJob(id: string): Job {
  return { id, type: 'upload_import', status: 'pending', submitted_at: '2026-09-07T00:00:00Z' } as Job;
}
''',
    '''function pendingJob(id: string): Job & BackgroundOperation {
  return {
    id,
    type: 'upload_import',
    kind: 'upload_import',
    status: 'pending',
    progress: 0,
    progress_total: 1,
    progress_completed: 0,
    progress_failed: 0,
    submitted_at: '2026-09-07T00:00:00Z',
    created_at: '2026-09-07T00:00:00Z'
  } as Job & BackgroundOperation;
}
''',
)
