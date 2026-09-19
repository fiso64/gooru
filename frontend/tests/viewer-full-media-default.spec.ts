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

async function mockApp(page: Page, loadFullMediaByDefault: boolean, capabilities: string[] = ['preview_images']) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ load_full_media_by_default: loadFullMediaByDefault, capabilities })
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
  await page.route('**/api/v1/files/one/content', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('configured full-media default starts on original and can be disabled in the viewer', async ({ page }) => {
  await mockApp(page, true);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const originalToggle = page.getByRole('button', { name: 'Use derived preview' });
  await expect(originalToggle).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('.viewer-visual-media')).toHaveAttribute('src', '/api/v1/files/one/content');

  await originalToggle.click();
  await expect(page.getByRole('button', { name: 'Use original media' })).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('.viewer-visual-media')).toHaveAttribute('src', '/api/v1/files/one/preview');
});

test('full-media default remains off when the runtime config is false', async ({ page }) => {
  await mockApp(page, false);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.getByRole('button', { name: 'Use original media' })).toHaveAttribute('aria-pressed', 'false');
  await expect(page.locator('.viewer-visual-media')).toHaveAttribute('src', '/api/v1/files/one/preview');
});

test('disabled preview capability forces original media and removes the viewer toggle', async ({ page }) => {
  await mockApp(page, false, []);

  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.locator('.viewer-visual-media')).toHaveAttribute('src', '/api/v1/files/one/content');
  await expect(page.getByRole('button', { name: 'Use original media' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Use derived preview' })).toHaveCount(0);

  await page.keyboard.press('q');
  await expect(page.locator('.viewer-visual-media')).toHaveAttribute('src', '/api/v1/files/one/content');
});
