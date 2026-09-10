from pathlib import Path
import re


def load(path):
    return Path(path).read_text(encoding="utf-8")


def save(path, text):
    Path(path).write_text(text, encoding="utf-8")


def remove_go_func(text, name):
    pattern = re.compile(r"\nfunc(?: \([^\n]*\))? " + re.escape(name) + r"\([^\n]*\)[^{]*\{.*?(?=\nfunc |\n(?:type|var|const) |\Z)", re.S)
    text, count = pattern.subn("\n", text, count=1)
    if count != 1:
        raise RuntimeError(f"expected Go function {name} once, found {count}")
    return text


def remove_ts_it(text, title):
    pattern = re.compile(r"\n  it\('" + re.escape(title) + r"'.*?\n  \}\);\n", re.S)
    text, count = pattern.subn("\n", text, count=1)
    if count != 1:
        raise RuntimeError(f"expected TS test {title!r} once, found {count}")
    return text


def remove_ts_function(text, name):
    pattern = re.compile(r"\n  (?:async )?function " + re.escape(name) + r"\([^\n]*\)[^{]*\{.*?(?=\n  (?:async )?function |\n  \$effect|\n  const |\n  let |\n</script>)", re.S)
    text, count = pattern.subn("\n", text, count=1)
    if count != 1:
        raise RuntimeError(f"expected TS function {name} once, found {count}")
    return text


def prune_go_imports(path):
    text = load(path)
    match = re.search(r"import \(\n(?P<body>.*?)\n\)\n", text, re.S)
    if not match:
        return
    rest = text[match.end():]
    kept = []
    aliases = {
        "encoding/json": "json",
        "net/http": "http",
        "net/http/httptest": "httptest",
        "path/filepath": "filepath",
    }
    for line in match.group("body").splitlines():
        m = re.match(r'\s*"([^"]+)"\s*$', line)
        if not m:
            kept.append(line)
            continue
        package = aliases.get(m.group(1), m.group(1).split("/")[-1])
        if package + "." in rest:
            kept.append(line)
    replacement = "import (\n" + "\n".join(kept) + "\n)\n"
    save(path, text[:match.start()] + replacement + text[match.end():])


for path in [
    "internal/serve/jobs.go",
    "internal/serve/jobs_batch.go",
    "internal/serve/jobs_batch_test.go",
    "internal/serve/jobs_history_response_test.go",
    "internal/serve/jobs_logging_test.go",
    "internal/serve/jobs_pagination.go",
    "internal/serve/jobs_pagination_test.go",
    "internal/serve/job_failure_diagnostics.go",
    "internal/serve/job_failure_diagnostics_test.go",
]:
    p = Path(path)
    if not p.exists():
        raise RuntimeError(f"missing expected legacy file {path}")
    p.unlink()

path = "internal/serve/server.go"
text = load(path)
text = "".join(line for line in text.splitlines(True) if "jobs                 *JobManager" not in line)
text = "".join(line for line in text.splitlines(True) if not ("jobs:" in line and "NewJobManagerWithLimits(" in line))
text = "".join(line for line in text.splitlines(True) if "/api/v1/jobs" not in line)
text, count = re.subn(r"\ntype JobListResponse struct \{.*?\n\}\n", "\n", text, count=1, flags=re.S)
if count != 1:
    raise RuntimeError("expected JobListResponse once")
for name in ["handleJobs", "handleJob", "handleGetJob", "handleCancelJob", "writeJobSubmitError"]:
    text = remove_go_func(text, name)
save(path, text)

path = "internal/serve/server_test.go"
text = load(path)
for name in [
    "TestJobRoutesRequireSession",
    "TestJobRoutesAcceptSessionCookie",
    "TestJobCollectionListsAndClearsFinishedJobs",
    "TestJobsRunSerially",
    "TestJobsRespectMaxRunning",
    "TestJobQueueFullReturnsStableError",
    "TestOversizedAsyncJobResultIsOmittedWithoutChangingSuccess",
    "TestOversizedSyncJobReturnsResultButDoesNotRetainIt",
    "TestAsyncJobSurvivesSubmittingContextCancellation",
    "TestSyncJobCancelsWithSubmittingContext",
    "TestCancelPendingJobSkipsRun",
    "TestSyncPendingJobCanceledByRequestContextSkipsRun",
    "TestCompletedJobsExpireAfterTTL",
    "waitForStatus",
    "waitForJobType",
    "saturateJobQueue",
]:
    text = remove_go_func(text, name)
save(path, text)
prune_go_imports(path)

path = "internal/serve/authorization_test.go"
text = load(path)
text = "".join(line for line in text.splitlines(True) if "/api/v1/jobs" not in line)
save(path, text)

path = "internal/serve/config.go"
text = load(path)
text, n = re.subn(r'^\s*Jobs\s+JobsConfig\s+`yaml:"jobs"`\n', "", text, count=1, flags=re.M)
if n != 1:
    raise RuntimeError("expected Config.Jobs once")
text, n = re.subn(r"\ntype JobsConfig struct \{.*?\n\}\n", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected JobsConfig once")
text, n = re.subn(r"\n\t\tJobs: JobsConfig\{.*?\n\t\t\},", "", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected default JobsConfig once")
text, n = re.subn(r'\n\tif cfg\.Jobs\.CompletedTTLRaw == "" \{.*?(?=\n\tif cfg\.Uploads\.MaxQueued <= 0)', "", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected jobs validation block once")
save(path, text)

path = "internal/serve/config_test.go"
text = load(path)
text, n = re.subn(r"\n\tif cfg\.Jobs\.CompletedTTL == 0 \{.*?\n\t\}\n\tif cfg\.Jobs\.MaxQueued != 100 \|\| cfg\.Jobs\.MaxRunning != 2 \|\| cfg\.Jobs\.MaxResultBytes != 10<<20 \{.*?\n\t\}\n", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected job default assertions once")
text = re.sub(r"\njobs:\n(?:  [^\n]*\n)+", "\n", text)
save(path, text)

path = "internal/background/runner.go"
text = load(path).replace(
    "// Run recovers expired work once at startup and then continuously claims work from one\n// resource class until ctx is canceled. Existing HTTP JobManager behavior is intentionally\n// outside this boundary; callers can migrate producers/consumers independently.\n",
    "// Run recovers expired work once at startup and then continuously claims work from one\n// resource class until ctx is canceled. Resource-class ownership keeps scheduling concerns\n// independent from the user-visible durable operation lifecycle.\n",
)
save(path, text)

path = "docs/CONFIG.md"
text = load(path)
text, n = re.subn(r"\n## `jobs`\n.*?(?=\n## `tools`)", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected CONFIG jobs section once")
text = re.sub(r"\njobs:\n(?:  [^\n]*\n)+", "\n", text)
marker = "| `uploads.max_file_size_bytes` | `0` | Optional upload per-file size setting. A zero value leaves the upload-specific size limit unset; set this explicitly when deployments need a hard upload cap. The generic `server.max_request_body_bytes` limit does not cap `/uploads`. |\n"
if marker not in text:
    raise RuntimeError("uploads.max_file_size_bytes docs marker missing")
text = text.replace(marker, marker + "| `uploads.max_queued` | `100` | Maximum number of durable upload operations admitted but not yet completed. Must be positive. |\n")
text = text.replace("| `uploads.conflict_policy` | `skip` | Default same-name behavior: `skip`, `rename`, `replace`, or `error`. |", "| `uploads.conflict_policy` | `rename` | Default same-name behavior: `skip`, `rename`, `replace`, or `error`. |")
text = text.replace("  max_file_size_bytes: 104857600\n  preserve_modtime:", "  max_file_size_bytes: 104857600\n  max_queued: 100\n  preserve_modtime:")
save(path, text)

path = "scripts/devprofile/main.go"
text = load(path)
text = re.sub(r"\njobs:\n(?:  [^\n]*\n)+", "\n", text)
text = text.replace("  max_file_size_bytes: 104857600\n  conflict_policy:", "  max_file_size_bytes: 104857600\n  max_queued: 100\n  conflict_policy:")
save(path, text)

path = "docs/openapi.yaml"
text = load(path)
text, n = re.subn(r"\n  /jobs:\n.*?(?=\ncomponents:\n)", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected /jobs API section once")
text, n = re.subn(r"\n    AsyncJob:\n.*?(?=\n    UnsupportedMedia:)", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected AsyncJob response once")
text, n = re.subn(r"\n    Job:\n.*?(?=\n    BrowserURLState:)", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected Job schemas once")
text = text.replace("poll the returned durable operation or legacy job as documented by the endpoint.", "poll the returned durable operation as documented by the endpoint.")
text = text.replace("Service is temporarily unavailable, including a full job queue.", "Service is temporarily unavailable, including a full durable-operation admission window.")
save(path, text)

path = "frontend/src/lib/api/types.ts"
text = load(path)
text = re.sub(r"^export type Job = .*\n", "", text, flags=re.M)
text = re.sub(r"^export type JobListResponse = .*\n", "", text, flags=re.M)
marker = "export type TagMutationResponse = components['schemas']['TagMutationResponse'];\n"
if marker not in text:
    raise RuntimeError("types TagMutationResponse marker missing")
view_model = "export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'canceled';\nexport interface Job {\n  id: string;\n  type: string;\n  status: JobStatus;\n  progress?: number;\n  submitted_at: string;\n  started_at?: string;\n  finished_at?: string;\n  result?: unknown;\n  error?: string;\n}\n"
text = text.replace(marker, marker + view_model)
save(path, text)

path = "frontend/src/lib/api/client.ts"
text = load(path)
text = text.replace("  Job,\n", "").replace("  JobListResponse,\n", "")
text = re.sub(r"^type JobStatus = .*\n", "", text, flags=re.M)
text = re.sub(r"^type ClearableJobStatus = .*\n", "", text, flags=re.M)
for name in ["getJob", "listJobs", "cancelJob", "clearJobs"]:
    pattern = re.compile(r"\n  async " + name + r"\([^\n]*\).*?\n  \}\n", re.S)
    text, n = pattern.subn("\n", text, count=1)
    if n != 1:
        raise RuntimeError(f"expected ApiClient.{name} once")
save(path, text)

path = "frontend/src/lib/api/client.test.ts"
text = remove_ts_it(load(path), "fetches jobs with cookies and cancels with CSRF")
save(path, text)

path = "frontend/src/lib/queries/jobs.ts"
text = load(path)
text = text.replace("import { ApiClient } from '$lib/api/client';\n", "")
text, n = re.subn(r"\n// Tag mutation is the final legacy JobManager producer\..*\Z", "\n", text, count=1, flags=re.S)
if n != 1:
    raise RuntimeError("expected legacy clear-jobs tail once")
text = text.rstrip() + "\n"
save(path, text)

path = "frontend/src/lib/components/AuthenticatedApp.svelte"
text = load(path)
text = text.replace("createCancelJobMutation, createClearJobsMutation, createJobQuery", "createCancelJobMutation, createJobQuery")
text = re.sub(r"^  const clearJobsMutation = .*\n", "", text, count=1, flags=re.M)
text = remove_ts_function(text, "clearCompletedJobs")
text = text.replace(" onClearCompleted={clearCompletedJobs}", "")
save(path, text)

path = "frontend/src/lib/components/JobsView.svelte"
text = load(path)
text = text.replace("    onCancel,\n    onClearCompleted\n", "    onCancel\n")
text = text.replace("    onCancel: (job: Job) => void;\n    onClearCompleted: () => void;\n", "    onCancel: (job: Job) => void;\n")
text = re.sub(r"  // Retain the prop.*?\n  void onClearCompleted;\n", "", text, count=1, flags=re.S)
save(path, text)

path = "spec/frontend.md"
text = load(path).replace("GET /api/v1/jobs/{id}\nDELETE /api/v1/jobs/{id}", "GET /api/v1/operations/{id}\nDELETE /api/v1/operations/{id}")
save(path, text)

path = "spec/operation/serve_and_api.md"
text = load(path)
text, n = re.subn(
    r"### 4\.3\. Job Management API\n.*?(?=\n### 4\.4\.)",
    "### 4.3. Durable Operation API\n\nLong-running user-visible work is represented by durable operations. Clients can list operations, inspect one operation, and cancel active work through `/api/v1/operations`. Operation state and aggregate progress survive server restarts; internal task/attempt rows are not exposed as top-level jobs.\n",
    text,
    count=1,
    flags=re.S,
)
if n != 1:
    raise RuntimeError("expected spec 4.3 once")
text, n = re.subn(
    r"#### Jobs \(Long-Running Operations\)\n.*?(?=\n## 5\.)",
    "#### Durable Operations\n\n*   `GET /api/v1/operations`: Lists visible durable operations.\n*   `GET /api/v1/operations/{operation_id}`: Gets aggregate durable operation state and a completed result when available.\n*   `DELETE /api/v1/operations/{operation_id}`: Cancels active operation work where possible.\n\n",
    text,
    count=1,
    flags=re.S,
)
if n != 1:
    raise RuntimeError("expected Jobs endpoint section once")
text = text.replace(
    "*   **Server Crash:** If the `gooru serve` process crashes, all in-memory state (including the job queue) is lost. Running jobs (goroutines) are terminated.\n*   **Database Locking:** The in-memory job manager bounds mutation concurrency but is not a durable queue. Later database/session work may replace parts of this model.",
    "*   **Server Crash:** Durable operation/task state survives process crashes and is reconciled on restart; expired leases are recovered and retryable work can continue safely.\n*   **Database Locking:** Durable admission and resource-class scheduling bound backlog/execution without depending on an in-memory queue.",
)
save(path, text)
