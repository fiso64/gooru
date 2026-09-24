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
  metadata: { image_width: 1600, image_height: 1200 },
  tags: [],
  media_urls: {
    thumbnail: '/api/v1/files/one/thumbnail',
    preview: '/api/v1/files/one/preview',
    content: '/api/v1/files/one/content',
    download: '/api/v1/files/one/download'
  }
};

async function mockApp(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ load_full_media_by_default: false })
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
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1200"><rect width="1600" height="1200"/></svg>';
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/one/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/one/content', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function openViewer(page: Page) {
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const image = page.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  return image;
}

test('viewer restores original-media, rotation, and fit state but not zoom after close and reopen', async ({ page }) => {
  await mockApp(page);
  let image = await openViewer(page);

  await page.getByRole('button', { name: 'Use original media' }).click();
  await expect(page.getByRole('button', { name: 'Use derived preview' })).toHaveAttribute('aria-pressed', 'true');
  await expect(image).toHaveAttribute('src', '/api/v1/files/one/content');

  const stage = page.locator('.viewer-stage');
  await stage.focus();
  await page.keyboard.press('2');
  await page.keyboard.press('r');
  await expect(page.locator('.viewer-mode-button[aria-label="Actual size"]')).toHaveClass(/active/);
  await expect(image).toHaveAttribute('style', /rotate\(90deg\)/);

  const box = await stage.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.keyboard.down('Control');
  await page.mouse.wheel(0, -240);
  await page.keyboard.up('Control');
  await expect(image).toHaveAttribute('style', /scale\((?!1(?:\.0+)?\))/);

  const stored = await page.evaluate(() => JSON.parse(sessionStorage.getItem('gooru.viewer.preferences.v1') ?? '{}'));
  expect(stored).toEqual({ version: 1, preferOriginal: true, fitMode: 'actual', rotation: 90 });
  expect(stored).not.toHaveProperty('zoom');
  expect(stored).not.toHaveProperty('fileID');

  await page.getByRole('button', { name: 'Close preview' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);

  image = await openViewer(page);
  await expect(page.getByRole('button', { name: 'Use derived preview' })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('.viewer-mode-button[aria-label="Actual size"]')).toHaveClass(/active/);
  await expect(image).toHaveAttribute('src', '/api/v1/files/one/content');
  await expect(image).toHaveAttribute('style', /rotate\(90deg\).*scale\(1\)/);
});
