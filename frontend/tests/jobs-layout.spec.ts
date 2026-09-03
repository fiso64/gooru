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
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'job-1', type: 'upload_import', status: 'running', progress: 0.5, submitted_at: '2026-09-02T08:00:00Z' }] })
  }));
}

async function login(page: Page) {
  await mockApp(page);
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('jobs view stays compact and left-aligns row content', async ({ page }) => {
  await login(page);
  await page.getByRole('complementary').getByRole('button', { name: 'Jobs' }).click();
  await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();

  const mainBox = await page.locator('main.main').boundingBox();
  const pageBox = await page.locator('.jobs-page').boundingBox();
  const cardBox = await page.locator('.jobs-card').boundingBox();
  const nameBox = await page.locator('.job-row .name').boundingBox();
  expect(mainBox).not.toBeNull();
  expect(pageBox).not.toBeNull();
  expect(cardBox).not.toBeNull();
  expect(nameBox).not.toBeNull();
  expect(pageBox!.width).toBeLessThanOrEqual(600.5);
  expect(pageBox!.x - mainBox!.x).toBeLessThan(48);
  expect(nameBox!.x - cardBox!.x).toBeLessThan(32);
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
