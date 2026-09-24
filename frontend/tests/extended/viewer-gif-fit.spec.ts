import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const gif = {
  id: 'tiny-gif',
  content_id: 'hash-tiny-gif',
  name: 'tiny.gif',
  safe_display_path: 'library/tiny.gif',
  size: 1024,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'image/gif',
  media_kind: 'gif',
  metadata: { image_width: 40, image_height: 20 },
  tags: [],
  media_urls: {
    thumbnail: '/api/v1/files/tiny-gif/thumbnail',
    preview: '/api/v1/files/tiny-gif/preview',
    content: '/api/v1/files/tiny-gif/content',
    download: '/api/v1/files/tiny-gif/download'
  }
};

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ load_full_media_by_default: false })
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
    body: JSON.stringify({ files: [gif], total_count: 1, library_count: 1, facets: { kind: [{ value: 'gif', count: 1 }] } })
  }));
  await page.route('**/api/v1/files/tiny-gif/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20"/></svg>'
  }));
  await page.route('**/api/v1/files/tiny-gif/content', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20"/></svg>'
  }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('GIF fit-to-screen scales like other media while actual size stays 1:1', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview tiny.gif' }).click();

  const stage = page.locator('.viewer-stage');
  const media = page.locator('.viewer-visual-media');
  await expect(stage).toBeVisible();
  await expect(media).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/tiny-gif\/content/);

  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().width)).toBeGreaterThan(40);
  await stage.focus();
  await page.keyboard.press('2');
  await expect.poll(() => media.evaluate((node) => Math.round(node.getBoundingClientRect().width))).toBe(40);
  await page.keyboard.press('1');
  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().width)).toBeGreaterThan(40);
});
