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

async function mockApp(page: Page) {
  const operations = Array.from({ length: 120 }, (_, index) => operation(index)).reverse();
  const requests: number[] = [];

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
    requests.push(limit);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: operations.slice(0, limit) }) });
  });

  return requests;
}

test('drawer stays bounded while jobs tab pages through large durable operation history', async ({ page }) => {
  const requests = await mockApp(page);
  await page.goto('/');

  const topbarJobs = page.locator('.topbar-right').getByRole('button', { name: 'Jobs' });
  await topbarJobs.click();
  const drawer = page.locator('#jobs-drawer .jobs-drawer');
  await expect(drawer).toBeVisible();
  await expect(drawer.locator('.job-row')).toHaveCount(20);
  await expect.poll(() => requests.includes(21)).toBe(true);
  expect(requests).not.toContain(1000);

  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
  await expect(page.getByText('Page 1', { exact: true })).toBeVisible();
  await expect.poll(() => requests.includes(51)).toBe(true);

  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 2', { exact: true })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
  await expect.poll(() => requests.includes(101)).toBe(true);

  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 3', { exact: true })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(20);
  await expect(page.getByRole('button', { name: 'Next', exact: true })).toBeDisabled();
  await expect.poll(() => requests.includes(151)).toBe(true);
  expect(requests).not.toContain(1000);

  await page.getByRole('button', { name: 'Previous', exact: true }).click();
  await expect(page.getByText('Page 2', { exact: true })).toBeVisible();
  await expect(page.locator('.jobs-card .job-row')).toHaveCount(50);
});
