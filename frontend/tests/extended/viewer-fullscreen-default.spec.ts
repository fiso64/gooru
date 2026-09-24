import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const file = {
  id: 'one',
  content_id: 'hash-one',
  name: 'one.jpg',
  safe_display_path: 'library/one.jpg',
  size: 2048,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'image/jpeg',
  media_kind: 'photo',
  metadata: { image_width: 800, image_height: 600 },
  tags: [],
  media_urls: {
    thumbnail: '/api/v1/files/one/thumbnail',
    preview: '/api/v1/files/one/preview',
    content: '/api/v1/files/one/content',
    download: '/api/v1/files/one/download'
  }
};

async function mockApp(page: Page, fullscreenMediaByDefault: boolean) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ fullscreen_media_by_default: fullscreenMediaByDefault })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>';
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/one/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function viewerIsFullscreen(page: Page) {
  return page.evaluate(() => document.fullscreenElement?.classList.contains('viewer-stage') ?? false);
}

test('configured fullscreen default requests fullscreen from the viewer-opening user gesture', async ({ page }) => {
  await mockApp(page, true);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.locator('.viewer-stage')).toBeVisible();
  await expect.poll(() => viewerIsFullscreen(page)).toBe(true);

  await page.keyboard.press('Escape');
  await expect.poll(() => page.evaluate(() => document.fullscreenElement === null)).toBe(true);
  await expect(page.locator('.viewer-stage')).toHaveCount(0);
});

test('F exits automatic fullscreen while keeping the normal viewer open', async ({ page }) => {
  await mockApp(page, true);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.locator('.viewer-stage')).toBeVisible();
  await expect.poll(() => viewerIsFullscreen(page)).toBe(true);

  await page.keyboard.press('f');
  await expect.poll(() => page.evaluate(() => document.fullscreenElement === null)).toBe(true);
  await expect(page.locator('.viewer-stage')).toBeVisible();
});

test('F exits fullscreen entered from the normal viewer even if fullscreen drops modal focus', async ({ page }) => {
  await mockApp(page, false);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.locator('.viewer-stage')).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.fullscreenElement === null)).toBe(true);

  await page.keyboard.press('f');
  await expect.poll(() => viewerIsFullscreen(page)).toBe(true);

  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await page.keyboard.press('f');
  await expect.poll(() => page.evaluate(() => document.fullscreenElement === null)).toBe(true);
  await expect(page.locator('.viewer-stage')).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'Search library' })).not.toBeFocused();
});

test('fullscreen default remains off when runtime config is false', async ({ page }) => {
  await mockApp(page, false);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.locator('.viewer-stage')).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.fullscreenElement === null)).toBe(true);
});
