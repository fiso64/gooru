import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockUIConfig(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ font_style: 'editorial', load_full_media_by_default: false })
  }));
}

async function mockAuthenticatedApp(page: Page) {
  await mockUIConfig(page);
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [], library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/operations?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], active_count: 0 }) }));
}

test('login footer links underline on hover', async ({ page }) => {
  await mockUIConfig(page);
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: 401,
    contentType: 'application/json',
    body: JSON.stringify({ error: { code: 'unauthorized', message: 'login required' } })
  }));

  await page.goto('/');
  const docs = page.locator('.login-v2-foot-links').getByRole('link', { name: 'docs' });
  await expect(docs).toBeVisible();
  await docs.hover();
  await expect(docs).toHaveCSS('text-decoration-line', 'underline');
});

test('jobs page keeps its heading on one line with room for header actions', async ({ page }) => {
  await mockAuthenticatedApp(page);
  await page.goto('/');
  await page.locator('.sidebar-item').filter({ hasText: 'Jobs' }).click();

  const pageContainer = page.locator('.jobs-page');
  const heading = pageContainer.getByRole('heading', { name: 'Background work' });
  await expect(heading).toBeVisible();
  await expect(heading).toHaveCSS('white-space', 'nowrap');
  await expect.poll(async () => (await pageContainer.boundingBox())?.width ?? 0).toBeGreaterThan(650);
});
