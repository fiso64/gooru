import { expect, test, type Page } from '@playwright/test';

const accent = '#0c2238';
const accentRgb = 'rgb(12, 34, 56)';
const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ accent_color: accent, load_full_media_by_default: false })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));

  await page.goto('/');
  await expect(page.getByLabel('Username')).toBeVisible();
}

test('runtime accent colors login branding, app logo spirals, and favicon', async ({ page }) => {
  await mockApp(page);

  await expect(page.locator('.login-v2-spirals path')).toHaveCSS('stroke', accentRgb);
  await expect(page.locator('.gooru-logo-accent-fill')).toHaveCSS('fill', accentRgb);

  const favicon = page.locator('link[rel="icon"][href^="data:image/svg+xml"]').last();
  await expect(favicon).toHaveCount(1);
  const faviconHref = await favicon.getAttribute('href');
  expect(faviconHref).not.toBeNull();
  expect(decodeURIComponent(faviconHref!.slice(faviconHref!.indexOf(',') + 1))).toContain(accent);

  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.locator('.gooru-logo-accent-fill')).toHaveCSS('fill', accentRgb);
});
