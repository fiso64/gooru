import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({ status: loggedIn ? 200 : 401, contentType: 'application/json', body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } }) });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/operations?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'op-1', kind: 'delete_files', status: 'running', progress_total: 2, progress_completed: 1, progress_failed: 0, created_at: '2026-09-02T08:00:00Z' }] })
  }));
}

async function login(page: Page) {
  await mockApp(page);
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('jobs view stays compact and aligns the header and status to the card edges', async ({ page }) => {
  await login(page);
  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
  const heading = page.getByRole('heading', { name: 'Background work' });
  await expect(heading).toBeVisible();

  const mainBox = await page.locator('main.main').boundingBox();
  const pageBox = await page.locator('.jobs-page').boundingBox();
  const cardBox = await page.locator('.jobs-card').boundingBox();
  const headingBox = await heading.boundingBox();
  const nameBox = await page.locator('.job-row .name').boundingBox();
  const statusBox = await page.locator('.job-row .status').boundingBox();
  expect(mainBox).not.toBeNull();
  expect(pageBox).not.toBeNull();
  expect(cardBox).not.toBeNull();
  expect(headingBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(statusBox).not.toBeNull();
  expect(pageBox!.width).toBeLessThanOrEqual(600.5);
  expect(pageBox!.x - mainBox!.x).toBeLessThan(48);
  expect(Math.abs(headingBox!.x - cardBox!.x)).toBeLessThan(2);
  expect(nameBox!.x - cardBox!.x).toBeLessThan(32);
  expect(cardBox!.x + cardBox!.width - (statusBox!.x + statusBox!.width)).toBeLessThan(32);

  await page.locator('.job-row').hover();
  await expect(page.getByRole('button', { name: 'Cancel Delete Files' })).toHaveCSS('opacity', '1');
  await expect(page.locator('.job-row .status')).toBeVisible();
});

test('jobs drawer overlays the library without changing page geometry', async ({ page }) => {
  await login(page);

  const main = page.locator('main.main');
  const jobsButton = page.locator('.topbar-right').getByRole('button', { name: 'Jobs' });
  const before = await main.boundingBox();
  const viewportWidth = await page.evaluate(() => window.innerWidth);
  const scrollWidthBefore = await page.evaluate(() => document.documentElement.scrollWidth);
  expect(before).not.toBeNull();

  await jobsButton.click();
  const drawer = page.locator('#jobs-drawer .jobs-drawer');
  await expect(drawer).toBeVisible();

  const after = await main.boundingBox();
  const drawerBox = await drawer.boundingBox();
  const scrollWidthAfter = await page.evaluate(() => document.documentElement.scrollWidth);
  expect(after).not.toBeNull();
  expect(drawerBox).not.toBeNull();
  expect(Math.abs(after!.x - before!.x)).toBeLessThan(0.5);
  expect(Math.abs(after!.width - before!.width)).toBeLessThan(0.5);
  expect(scrollWidthAfter).toBe(scrollWidthBefore);
  expect(scrollWidthAfter).toBeLessThanOrEqual(viewportWidth);
  expect(drawerBox!.x).toBeLessThan(after!.x + after!.width);
});
