from pathlib import Path
import re


def read(path: str) -> str:
    return Path(path).read_text(encoding='utf-8')


def write(path: str, text: str) -> None:
    Path(path).write_text(text.rstrip() + '\n', encoding='utf-8')


# Remove the redundant old-transport fixture from the metadata-refresh test;
# this test already has an operations route that counts durable polling.
path = 'frontend/tests/upload-metadata-refresh.spec.ts'
text = read(path)
old = """  await page.route('**/api/v1/jobs**', async (route) => {\n    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });\n  });\n"""
if old not in text:
    raise RuntimeError('metadata-refresh legacy jobs fixture not found')
write(path, text.replace(old, ''))

# Migrate only legacy browser routes. Exact collection job fixtures become a
# query-capable durable collection route because the Jobs UI supplies ?limit;
# existing exact /operations fixtures are left untouched.
for p in Path('frontend/tests').glob('*.spec.ts'):
    text = p.read_text(encoding='utf-8')
    text = text.replace("'**/api/v1/jobs'", "'**/api/v1/operations?**'")
    text = text.replace('"**/api/v1/jobs"', '"**/api/v1/operations?**"')
    text = text.replace('/api/v1/jobs', '/api/v1/operations')
    p.write_text(text, encoding='utf-8')

# Tests that model upload status transitions need actual durable operation DTOs.
for path in [
    'frontend/tests/upload-admission-throughput.spec.ts',
    'frontend/tests/upload-backpressure.spec.ts',
    'frontend/tests/upload-refresh-throttle.spec.ts',
]:
    text = read(path)
    text = text.replace('function jobResponse(', 'function operationResponse(')
    text = text.replace('jobResponse(', 'operationResponse(')
    old_fields = """    type: 'upload_import',\n    status: completed ? 'completed' : 'pending',\n    progress: completed ? 1 : 0,\n    submitted_at: '2026-09-07T00:00:00Z',\n"""
    new_fields = """    kind: 'upload_import',\n    status: completed ? 'completed' : 'pending',\n    progress_total: 1,\n    progress_completed: completed ? 1 : 0,\n    progress_failed: 0,\n    created_at: '2026-09-07T00:00:00Z',\n"""
    if old_fields not in text:
        raise RuntimeError(f'{path}: old job response fields not found')
    text = text.replace(old_fields, new_fields)
    if path.endswith('upload-backpressure.spec.ts'):
        text = text.replace("test('job queue full is transient backpressure instead of a failed upload'", "test('durable admission saturation is transient backpressure instead of a failed upload'")
    write(path, text)

# Small upload-focused tests only need pending durable-operation admission.
for path in [
    'frontend/tests/upload-conflict-default.spec.ts',
    'frontend/tests/upload-isolation.spec.ts',
    'frontend/tests/upload-row-layout.spec.ts',
]:
    text = read(path)
    text = text.replace(
        "type: 'upload_import', status: 'pending', submitted_at:",
        "kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at:"
    )
    write(path, text)

# Shell: migrate the upload polling/cancellation scenario to the durable API.
path = 'frontend/tests/shell.spec.ts'
text = read(path)
text = text.replace("test('uploads with job polling and cancellation'", "test('uploads with durable operation polling and cancellation'")
start = text.index('  let canceled = false;\n', text.index("test('uploads with durable operation polling and cancellation'"))
end = text.index('\n\n  await page.goto(\'/\');', start)
replacement = '''  let canceled = false;
  await page.route('**/api/v1/operations?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    const operation = {
      id: 'job-one',
      kind: 'upload_import',
      status: canceled ? 'canceled' : 'running',
      progress_total: 10,
      progress_completed: canceled ? 10 : 4,
      progress_failed: 0,
      created_at: '2026-05-20T00:00:00Z'
    };
    const visible = !canceled && (!ids.length || ids.includes(operation.id));
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: visible ? [operation] : [] }) });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-05-20T00:00:00Z' })
    });
  });
  const cancelRequests: Array<{ csrf: string; method: string }> = [];
  await page.route('**/api/v1/operations/job-one', async (route) => {
    if (route.request().method() === 'DELETE') {
      cancelRequests.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', method: route.request().method() });
      canceled = true;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-one', kind: 'upload_import', status: 'canceled', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', finished_at: '2026-05-20T00:01:00Z' })
      });
    }
    return route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-one', kind: 'upload_import', status: canceled ? 'canceled' : 'running', progress_total: 10, progress_completed: canceled ? 10 : 5, progress_failed: 0, created_at: '2026-05-20T00:00:00Z' })
    });
  });'''
text = text[:start] + replacement + text[end:]

# Shell: the Jobs drawer/page now share durable cancellation and persistent
# history. Clearing history is deliberately absent.
marker = "test('Jobs drawer preserves route and shares real job actions with the page'"
start = text.index('  let canceled = false;\n', text.index(marker))
end = text.index("\n\n  await page.goto('/');", start)
replacement = '''  let canceled = false;
  const cancels: Array<{ csrf: string; method: string }> = [];

  await page.route('**/api/v1/operations**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/operations/job-run') && route.request().method() === 'DELETE') {
      cancels.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', method: route.request().method() });
      canceled = true;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-run', kind: 'upload_import', status: 'canceled', progress_total: 10, progress_completed: 4, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', finished_at: '2026-05-20T00:02:00Z' })
      });
    }
    if (url.pathname.endsWith('/operations')) {
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'job-run', kind: 'upload_import', status: canceled ? 'canceled' : 'running', progress_total: 10, progress_completed: 4, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
            { id: 'job-done', kind: 'tag_mutation', status: 'completed', progress_total: 1, progress_completed: 1, progress_failed: 0, created_at: '2026-05-19T23:00:00Z', started_at: '2026-05-19T23:01:00Z', finished_at: '2026-05-19T23:02:00Z' }
          ]
        })
      });
    }
    return route.fallback();
  });'''
text = text[:start] + replacement + text[end:]
old_clear = '''  await page.locator('.jobs-page-header').hover();
  await page.getByRole('button', { name: 'Clear completed' }).click();
  await expect.poll(() => clears).toEqual([{ csrf: 'csrf-one', status: 'completed' }]);
'''
if old_clear not in text:
    raise RuntimeError('shell legacy clear-history assertion not found')
text = text.replace(old_clear, "  await expect(page.getByRole('button', { name: 'Clear completed' })).toHaveCount(0);\n")

# Shell: utility-view fixture uses durable operation DTOs.
old_items = '''          { id: 'utility-run', type: 'upload_import', status: 'running', progress: 0.4, submitted_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
          { id: 'utility-done', type: 'bulk_tag', status: 'completed', progress: 1, submitted_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z', completed_at: '2026-05-20T00:02:00Z' }
'''
new_items = '''          { id: 'utility-run', kind: 'upload_import', status: 'running', progress_total: 10, progress_completed: 4, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
          { id: 'utility-done', kind: 'bulk_tag', status: 'completed', progress_total: 1, progress_completed: 1, progress_failed: 0, created_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z', finished_at: '2026-05-20T00:02:00Z' }
'''
if old_items not in text:
    raise RuntimeError('shell utility legacy DTOs not found')
text = text.replace(old_items, new_items)
write(path, text)

# Durable terminology in source/spec. Keep the stable public error code/message
# job_queue_full / "job queue is full" intact for existing clients.
path = 'frontend/src/lib/state/uploadWorkflow.svelte.ts'
text = read(path).replace(
    '// accepted unfinished imports to one batched status window.\n                // This keeps large batches from outrunning the server job queue.',
    '// accepted unfinished imports to one batched status window.\n                // This keeps large batches from outrunning the durable admission window.'
)
write(path, text)

path = 'spec/operation/serve_and_api.md'
text = read(path)
text = text.replace(
    'upload/import, thumbnails/previews, and in-memory jobs for a Gooru database.',
    'upload/import, thumbnails/previews, and durable background operations for a Gooru database.'
)
old_concurrency = '''*   **Concurrency Model: Bounded In-Memory Jobs**
    *   Mutations can run synchronously or asynchronously through an in-memory job manager.
    *   `jobs.max_queued` bounds pending work, `jobs.max_running` bounds concurrently running async jobs, `jobs.completed_ttl` expires terminal jobs, and `jobs.max_result_bytes` prevents large results from being retained.
    *   Read requests do not enter the job queue.
'''
new_concurrency = '''*   **Concurrency Model: Durable Background Operations**
    *   Long-running mutations are admitted as durable operations whose task state can survive process restart.
    *   Producer-specific admission limits bound unfinished work, while resource-class workers bound execution and lease tasks for retry/recovery.
    *   Read requests do not enter the durable background scheduler.
'''
if old_concurrency not in text:
    raise RuntimeError('legacy concurrency-model spec block not found')
text = text.replace(old_concurrency, new_concurrency)
text = text.replace(
    '    *   The request will be placed in the write queue, and the server will wait for the job to be completed before sending a response.\n',
    '    *   The server waits for the admitted durable operation to reach a terminal state and returns the domain result.\n',
    1
)
text = text.replace(
    '    *   The request will be placed in the write queue, and the server will immediately respond without waiting for the job to complete.\n    *   **Response:** `202 Accepted` with a Job object in the body, containing a unique `id` for polling.\n',
    '    *   The server durably admits the operation and immediately responds without waiting for completion.\n    *   **Response:** `202 Accepted` with a `BackgroundOperation` object containing a unique `id` for polling.\n'
)
write(path, text)
