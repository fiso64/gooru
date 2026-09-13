from pathlib import Path

def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if text.count(old) != 1:
        raise SystemExit(f'unexpected state in {path}: {old[:60]!r}')
    p.write_text(text.replace(old, new, 1))

replace('frontend/src/lib/components/AuthenticatedApp.svelte',
'''    jobsActiveCount={jobsQuery.data?.active_count ?? activeJobs.length}
    jobs={jobsQuery.data?.items ?? []}
    jobsDrawerOpen={jobsDrawerOpen}
''',
'''    jobsActiveCount={jobsQuery.data?.active_count ?? activeJobs.length}
    jobs={jobsQuery.data?.items ?? []}
    jobsTotalCount={jobsQuery.data?.total_count ?? (jobsQuery.data?.items.length ?? 0)}
    jobsDrawerOpen={jobsDrawerOpen}
''')

replace('frontend/src/lib/components/AppShell.svelte',
'''    jobsActiveCount,
    jobs,
    jobsDrawerOpen,
''',
'''    jobsActiveCount,
    jobs,
    jobsTotalCount,
    jobsDrawerOpen,
''')
replace('frontend/src/lib/components/AppShell.svelte',
'''    jobsActiveCount: number;
    jobs: Job[];
    jobsDrawerOpen: boolean;
''',
'''    jobsActiveCount: number;
    jobs: Job[];
    jobsTotalCount: number;
    jobsDrawerOpen: boolean;
''')
replace('frontend/src/lib/components/AppShell.svelte',
'''  function openLibrary() {
    if (route === 'library') onSearchCommit('');
    onRoute('library');
  }
''',
'''  function openLibrary() {
    if (route === 'library') onSearchCommit('');
    onRoute('library');
  }

  function openAllJobs() {
    onCloseJobs();
    onRoute('jobs');
  }
''')
replace('frontend/src/lib/components/AppShell.svelte',
'''    <div id="jobs-drawer"><JobsDrawer {jobs} onClose={onCloseJobs} onCancel={onCancelJob} /></div>
''',
'''    <div id="jobs-drawer"><JobsDrawer {jobs} totalCount={jobsTotalCount} onViewAll={openAllJobs} onClose={onCloseJobs} onCancel={onCancelJob} /></div>
''')

replace('frontend/src/lib/components/JobsDrawer.svelte',
'''    jobs,
    onClose,
    onCancel
  } = $props<{
    jobs: Job[];
    onClose: () => void;
''',
'''    jobs,
    totalCount,
    onViewAll,
    onClose,
    onCancel
  } = $props<{
    jobs: Job[];
    totalCount: number;
    onViewAll: () => void;
    onClose: () => void;
''')
replace('frontend/src/lib/components/JobsDrawer.svelte',
'''    {/each}
  </div>
</div>
''',
'''    {/each}
    {#if totalCount > jobs.length}
      <button class="jobs-view-all" type="button" data-testid="jobs-view-all" onclick={onViewAll}>View all</button>
    {/if}
  </div>
</div>
''')
replace('frontend/src/lib/components/JobsDrawer.svelte',
'''  @media (max-width: 700px) {
''',
'''  .jobs-view-all {
    width: 100%;
    border: 0;
    border-top: 1px solid var(--border);
    padding: 11px 14px;
    background: transparent;
    color: var(--accent);
    font-family: var(--font-mono);
    font-size: 11px;
    cursor: pointer;
    text-align: center;
  }

  .jobs-view-all:hover,
  .jobs-view-all:focus-visible {
    background: var(--bg-3);
  }

  @media (max-width: 700px) {
''')

replace('frontend/tests/jobs-pagination.spec.ts',
'''  await expect.poll(() => requested(requests, 20, 0)).toBe(true);
  expect(requests.every((request) => request.limit <= 50)).toBe(true);

  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
''',
'''  await expect.poll(() => requested(requests, 20, 0)).toBe(true);
  expect(requests.every((request) => request.limit <= 50)).toBe(true);
  const viewAll = drawer.getByRole('button', { name: 'View all' });
  await expect(viewAll).toBeVisible();

  await viewAll.click();
  await expect(drawer).toBeHidden();
''')
