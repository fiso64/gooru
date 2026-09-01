from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"{label} not found")
    return text.replace(old, new, 1)


app = Path("frontend/src/lib/components/AuthenticatedApp.svelte")
text = app.read_text()
text = replace_once(
    text,
    "  let cancelRequestedJobID = $state('');\n  let fileMetadata = $state<{",
    "  let cancelRequestedJobID = $state('');\n  let jobsDrawerOpen = $state(false);\n  let fileMetadata = $state<{",
    "jobs drawer state",
)
text = replace_once(
    text,
    "    fileMetadata = null;\n    cancelRequestedJobID = '';\n    closeActionDialog();",
    "    fileMetadata = null;\n    cancelRequestedJobID = '';\n    jobsDrawerOpen = false;\n    closeActionDialog();",
    "jobs drawer auth reset",
)
text = replace_once(
    text,
    "  function handleKeydown(event: KeyboardEvent) {\n    library.handleKeydown(event, loadedFiles);\n  }\n\n  function closeActionDialog() {",
    "  function handleKeydown(event: KeyboardEvent) {\n    library.handleKeydown(event, loadedFiles);\n  }\n\n  function setRoute(route: string) {\n    jobsDrawerOpen = false;\n    library.setRoute(route);\n  }\n\n  function toggleJobsDrawer() {\n    jobsDrawerOpen = !jobsDrawerOpen;\n  }\n\n  function closeJobsDrawer() {\n    jobsDrawerOpen = false;\n  }\n\n  function closeActionDialog() {",
    "jobs drawer helpers",
)
text = replace_once(
    text,
    "    jobsActiveCount={activeJobs.length}\n    kindCounts={page?.facets?.kind ?? []}",
    "    jobsActiveCount={activeJobs.length}\n    jobs={jobsQuery.data?.items ?? []}\n    jobsDrawerOpen={jobsDrawerOpen}\n    kindCounts={page?.facets?.kind ?? []}",
    "AppShell jobs props",
)
text = replace_once(text, "    onRoute={library.setRoute}", "    onRoute={setRoute}", "AppShell route wrapper")
text = replace_once(
    text,
    "    onSearchCommit={library.commitSearch}\n    onJobs={library.toggleJobsRoute}",
    "    onSearchCommit={library.commitSearch}\n    onJobs={toggleJobsDrawer}\n    onCloseJobs={closeJobsDrawer}\n    onCancelJob={cancelJob}",
    "AppShell Jobs callbacks",
)
app.write_text(text)

workflow = Path("frontend/src/lib/state/libraryWorkflow.svelte.ts")
text = workflow.read_text()
text = replace_once(
    text,
    "\n  function toggleJobsRoute() {\n    route = route === 'jobs' ? 'library' : 'jobs';\n  }\n",
    "\n",
    "obsolete Jobs route helper",
)
text = replace_once(
    text,
    "    handleKeydown,\n    setRoute,\n    toggleJobsRoute",
    "    handleKeydown,\n    setRoute",
    "obsolete Jobs route export",
)
workflow.write_text(text)

docs = Path("docs/issue-21-concept-discrepancies.md")
text = docs.read_text()
additions = "\n".join(
    [
        "29. Jobs page and top-bar drawer row rendering: B. Use one shared concept-shaped row component so status, progress, timestamps, and cancel behavior cannot drift between the page and drawer.",
        "30. Jobs pause-all control: A. The concept affordance remains visible in the drawer but disabled/coming soon because the backend has no pause/resume endpoint.",
        "31. Concept job `done / total` counters: C. The backend exposes a normalized progress ratio but not authoritative item totals, so the real UI must render percentage/progress and timestamps rather than invent prototype counts.",
        "32. Running-job cancel and clear-completed actions: B. Preserve the supported backend actions, but keep them visually subordinate (row/header hover or focus) so the resting Jobs surfaces remain faithful to the concept.",
    ]
)
if "29. Jobs page and top-bar drawer row rendering:" not in text:
    text = text.rstrip() + "\n" + additions + "\n"
docs.write_text(text)

spec = Path("frontend/tests/shell.spec.ts")
text = spec.read_text()
test_body = r'''

test('Jobs drawer preserves route and shares real job actions with the page', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });

  let canceled = false;
  const cancels: Array<{ csrf: string; method: string }> = [];
  const clears: Array<{ csrf: string; status: string }> = [];

  await page.route('**/api/v1/jobs**', async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith('/jobs/job-run') && route.request().method() === 'DELETE') {
      cancels.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', method: route.request().method() });
      canceled = true;
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ id: 'job-run', type: 'upload_import', status: 'canceled', progress: 0.4, submitted_at: '2026-05-20T00:00:00Z' })
      });
    }
    if (url.pathname.endsWith('/jobs') && route.request().method() === 'DELETE') {
      clears.push({ csrf: route.request().headers()['x-gooru-csrf'] ?? '', status: url.searchParams.get('status') ?? '' });
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ removed: 1 }) });
    }
    if (url.pathname.endsWith('/jobs')) {
      return route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          items: [
            { id: 'job-run', type: 'upload_import', status: canceled ? 'canceled' : 'running', progress: 0.4, submitted_at: '2026-05-20T00:00:00Z', started_at: '2026-05-20T00:01:00Z' },
            { id: 'job-done', type: 'tag_mutation', status: 'completed', progress: 1, submitted_at: '2026-05-19T23:00:00Z', started_at: '2026-05-19T23:01:00Z' }
          ]
        })
      });
    }
    return route.fallback();
  });

  await page.goto('/');
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  await page.locator('.topbar-right').getByRole('button', { name: 'Jobs' }).click();
  const drawer = page.getByRole('dialog', { name: 'Jobs' });
  await expect(drawer).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(drawer.getByText('Import media')).toBeVisible();
  await expect(drawer.getByRole('button', { name: 'Pause all coming soon' })).toBeDisabled();
  await drawer.locator('.job-row').filter({ hasText: 'Import media' }).hover();
  await drawer.getByRole('button', { name: 'Cancel Import media' }).click();
  await expect.poll(() => cancels).toEqual([{ csrf: 'csrf-one', method: 'DELETE' }]);

  await drawer.getByRole('button', { name: 'Close jobs' }).click();
  await expect(page.getByRole('dialog', { name: 'Jobs' })).toHaveCount(0);

  await page.locator('.sidebar').getByRole('button', { name: /Jobs/ }).click();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.getByText('Tag edit')).toBeVisible();
  await page.locator('.jobs-page-header').hover();
  await page.getByRole('button', { name: 'Clear completed' }).click();
  await expect.poll(() => clears).toEqual([{ csrf: 'csrf-one', status: 'completed' }]);
});
'''
if "test('Jobs drawer preserves route and shares real job actions with the page'" not in text:
    text += test_body
spec.write_text(text)
