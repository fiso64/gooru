import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function operation(index: number) {
  return {
    id: `operation-${index}`,
    kind: `history_${index}`,
    status: index < 7 ? 'running' : 'completed',
    progress_total: 10,
    progress_completed: index < 7 ? 5 : 10,
    progress_failed: 0,
    created_at: new Date(Date.UTC(2026, 8, 5, 12, 0, index)).toISOString(),
    finished_at: index < 7 ? undefined : new Date(Date.UTC(2026, 8, 5, 12, 0, index + 1)).toISOString()
  };
}

type OperationRequest = { limit: number; offset: number };

async function mockApp(page: Page) {
  const operations = Array.from({ length: 120 }, (_, index) => operation(index)).reverse();
  const requests: OperationRequest[] = [];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/operations?**', async (route) => {
    const url = new URL(route.request().url());
    const limit = Number(url.searchParams.get('limit') ?? '0');
    const offset = Number(url.searchParams.get('offset') ?? '0');
    requests.push({ limit, offset });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: operations.slice(offset, offset + limit),
        active_count: operations.filter((item) => item.status === 'pending' || item.status === 'running').length,
        total_count: operations.length
      })
    });
  });

  return requests;
}

function requested(requests: OperationRequest[], limit: number, offset: number) {
  return requests.some((request) => request.limit === limit && request.offset === offset);
}

test('drawer stays bounded while jobs tab uses true server paging above and below durable history', async ({ page }) => {
  const requests = await mockApp(page);
  await page.goto('/');

  const topbarJobs = page.locator('.topbar-right').getByRole('button', { name: 'Jobs' });
  await topbarJobs.click();
  const drawer = page.locator('#jobs-drawer .jobs-drawer');
  await expect(drawer).toBeVisible();
  await expect(drawer.locator('.job-row')).toHaveCount(20);
  await expect.poll(() => requested(requests, 50, 0)).toBe(true);
  expect(requests.some((request) => request.limit === 20)).toBe(false);
  expect(requests.every((request) => request.limit <= 50)).toBe(true);
  const viewAll = drawer.getByRole('button', { name: 'View all' });
  await expect(viewAll).toBeVisible();

  await viewAll.click();
  await expect(drawer).toBeHidden();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
  await expect.poll(() => requested(requests, 50, 0)).toBe(true);

  const topPager = page.getByTestId('jobs-pages-top');
  const bottomPager = page.getByTestId('jobs-pages-bottom');
  await expect(topPager).toBeVisible();
  await expect(bottomPager).toBeVisible();
  await expect(topPager.getByRole('button', { name: 'Page 1' })).toHaveAttribute('aria-current', 'page');
  await expect(bottomPager.getByRole('button', { name: 'Page 1' })).toHaveAttribute('aria-current', 'page');

  await topPager.getByRole('button', { name: 'Next page' }).click();
  await expect(topPager.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
  await expect(bottomPager.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
  await expect.poll(() => requested(requests, 50, 50)).toBe(true);

  await bottomPager.getByRole('button', { name: 'Next page' }).click();
  await expect(topPager.getByRole('button', { name: 'Page 3' })).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(20);
  await expect(bottomPager.getByRole('button', { name: 'Next page' })).toBeDisabled();
  await expect.poll(() => requested(requests, 50, 100)).toBe(true);

  await topPager.getByRole('button', { name: 'Previous page' }).click();
  await expect(bottomPager.getByRole('button', { name: 'Page 2' })).toHaveAttribute('aria-current', 'page');
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
});
