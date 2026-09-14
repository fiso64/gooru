import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type Operation = {
  id: string;
  kind: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'canceled';
  stage?: 'receiving' | 'importing';
  progress_total: number;
  progress_completed: number;
  progress_failed: number;
  created_at: string;
  error_message?: string;
};

async function mockApp(page: Page) {
  let operations: Operation[] = [
    {
      id: 'op-running',
      kind: 'delete_files',
      status: 'running',
      progress_total: 2,
      progress_completed: 1,
      progress_failed: 0,
      created_at: '2026-09-12T01:00:00Z'
    },
    {
      id: 'op-failed',
      kind: 'tag_edit',
      status: 'failed',
      progress_total: 3,
      progress_completed: 2,
      progress_failed: 1,
      created_at: '2026-09-12T00:59:00Z',
      error_message: 'one file failed'
    },
    {
      id: 'op-completed',
      kind: 'upload_import',
      stage: 'importing',
      status: 'completed',
      progress_total: 4,
      progress_completed: 4,
      progress_failed: 0,
      created_at: '2026-09-12T00:58:00Z'
    }
  ];
  const clears: string[] = [];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/operations**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    if (request.method() === 'DELETE' && url.pathname === '/api/v1/operations') {
      clears.push(request.headers()['x-gooru-csrf'] ?? '');
      const before = operations.length;
      operations = operations.filter((item) => item.status === 'pending' || item.status === 'running');
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ cleared: before - operations.length }) });
      return;
    }
    if (request.method() === 'GET' && url.pathname === '/api/v1/operations') {
      const limit = Number(url.searchParams.get('limit') ?? operations.length);
      const offset = Number(url.searchParams.get('offset') ?? '0');
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          items: operations.slice(offset, offset + limit),
          active_count: operations.filter((item) => item.status === 'pending' || item.status === 'running').length,
          total_count: operations.length
        })
      });
      return;
    }
    await route.abort();
  });

  return clears;
}

test('jobs history keeps status visible, shows file counts, and confirms terminal-history clearing', async ({ page }) => {
  const clears = await mockApp(page);
  await page.goto('/');
  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();

  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.getByText('Durable operation history remains available across restarts.')).toHaveCount(0);
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(3);

  const runningRow = page.locator('.jobs-card .job-row').filter({ hasText: 'Delete Files' });
  const failedRow = page.locator('.jobs-card .job-row').filter({ hasText: 'Tag edit' });
  const importRow = page.locator('.jobs-card .job-row').filter({ hasText: 'Import media' });
  await expect(runningRow.getByText('2 files', { exact: true })).toHaveCount(0);
  await expect(failedRow.getByText('3 files', { exact: true })).toHaveCount(0);
  await expect(importRow.getByText('4 files', { exact: true })).toBeVisible();

  const runningStatus = runningRow.locator('.status.running');
  await runningRow.hover();
  await expect(runningStatus).toHaveText('running');
  await expect.poll(async () => runningStatus.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');

  const runningBlue = await page.evaluate(() => {
    const status = document.querySelector<HTMLElement>('.jobs-card .status.running');
    const progress = document.querySelector<HTMLElement>('.jobs-card .job-progress.running > div');
    const probe = document.createElement('span');
    probe.style.color = 'oklch(0.72 0.15 250)';
    document.body.appendChild(probe);
    const expected = getComputedStyle(probe).color;
    probe.remove();
    return {
      status: status ? getComputedStyle(status).color : '',
      progress: progress ? getComputedStyle(progress).backgroundColor : '',
      expected
    };
  });
  expect(runningBlue.status).toBe(runningBlue.expected);
  expect(runningBlue.progress).toBe(runningBlue.expected);

  const rowBox = await runningRow.boundingBox();
  const statusBox = await runningStatus.boundingBox();
  expect(rowBox).not.toBeNull();
  expect(statusBox).not.toBeNull();
  expect(statusBox!.x).toBeGreaterThan(rowBox!.x + rowBox!.width * 0.55);

  await page.getByRole('button', { name: 'Clear completed', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Clear completed jobs' });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText('Running and queued jobs will not be affected.')).toBeVisible();
  await dialog.getByRole('button', { name: 'Clear jobs' }).click();

  await expect.poll(() => clears.length).toBe(1);
  expect(clears[0]).toBe('csrf-one');
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(1);
  await expect(page.locator('.jobs-card .job-row').filter({ hasText: 'Delete Files' })).toBeVisible();
  await expect(page.getByText('one file failed')).toHaveCount(0);
  await expect(page.getByText('Import media')).toHaveCount(0);

  const topbarJobs = page.locator('.topbar-right').getByRole('button', { name: 'Jobs' });
  await topbarJobs.click();
  const drawer = page.locator('#jobs-drawer .jobs-drawer');
  await expect(drawer).toBeVisible();
  await expect(drawer.getByRole('button', { name: 'Clear completed jobs' })).toBeVisible();
  await expect(drawer.getByRole('button', { name: 'Close jobs' })).toHaveCount(0);
  await drawer.getByRole('button', { name: 'Clear completed jobs' }).click();
  await expect(page.getByRole('dialog', { name: 'Clear completed jobs' })).toBeVisible();
});
