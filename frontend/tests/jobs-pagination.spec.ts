import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function job(index: number) {
  return {
    id: `job-${index}`,
    type: `history_${index}`,
    status: 'completed',
    progress: 1,
    submitted_at: new Date(Date.UTC(2026, 8, 5, 12, 0, index)).toISOString(),
    finished_at: new Date(Date.UTC(2026, 8, 5, 12, 0, index + 1)).toISOString()
  };
}

function offsetFromToken(token: string | null) {
  if (!token) return 0;
  const decoded = Buffer.from(token, 'base64url').toString('utf8');
  const match = /^offset:(\d+)$/.exec(decoded);
  return match ? Number(match[1]) : 0;
}

async function mockApp(page: Page) {
  const jobs = Array.from({ length: 120 }, (_, index) => job(index)).reverse();
  const requests: Array<{ limit: number; offset: number }> = [];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/jobs?**', async (route) => {
    const url = new URL(route.request().url());
    const limit = Number(url.searchParams.get('limit') ?? '50');
    const offset = offsetFromToken(url.searchParams.get('page_token'));
    requests.push({ limit, offset });
    const items = jobs.slice(offset, offset + limit);
    const nextOffset = offset + items.length;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items,
        active_count: 0,
        next_page_token: nextOffset < jobs.length ? Buffer.from(`offset:${nextOffset}`).toString('base64url') : undefined
      })
    });
  });

  return requests;
}

test('drawer stays bounded while jobs tab pages through large completed history', async ({ page }) => {
  const requests = await mockApp(page);
  await page.goto('/');

  const topbarJobs = page.locator('.topbar-right').getByRole('button', { name: 'Jobs' });
  await topbarJobs.click();
  const drawer = page.locator('#jobs-drawer .jobs-drawer');
  await expect(drawer).toBeVisible();
  await expect(drawer.locator('.job-row')).toHaveCount(20);
  await expect.poll(() => requests.some((request) => request.limit === 20 && request.offset === 0)).toBe(true);

  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
  await expect(page.getByText('Page 1', { exact: true })).toBeVisible();
  await expect.poll(() => requests.some((request) => request.limit === 50 && request.offset === 0)).toBe(true);

  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 2', { exact: true })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
  await expect.poll(() => requests.some((request) => request.limit === 50 && request.offset === 50)).toBe(true);

  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 3', { exact: true })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(20);
  await expect(page.getByRole('button', { name: 'Next', exact: true })).toBeDisabled();
  await expect.poll(() => requests.some((request) => request.limit === 50 && request.offset === 100)).toBe(true);

  await page.getByRole('button', { name: 'Previous', exact: true }).click();
  await expect(page.getByText('Page 2', { exact: true })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
});
