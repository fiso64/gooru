import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_fit', username: 'fit-user', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-fit'
};

async function openTinyAnimatedViewer(page: Page) {
  let loggedIn = false;
  const file = {
    id: 'tiny-gif', content_id: 'hash-tiny-gif', name: 'tiny.gif', safe_display_path: 'library/tiny.gif', size: 1024,
    added_at: '2026-09-01T00:00:00Z', modified_time: '2026-09-01T00:00:00Z', media_type: 'image/gif', media_kind: 'gif',
    metadata: { image_width: 320, image_height: 200 }, tags: [], can_delete: true,
    media_urls: {
      thumbnail: '/api/v1/files/tiny-gif/thumbnail', preview: '/api/v1/files/tiny-gif/preview',
      content: '/api/v1/files/tiny-gif/content', download: '/api/v1/files/tiny-gif/download'
    }
  };

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401, contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ viewer_fit_mode: 'original_size_if_fit', viewer_scaling: 'nearest' })
  }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="200"><rect width="320" height="200"/></svg>';
  for (const endpoint of ['thumbnail', 'preview', 'content']) {
    await page.route(`**/api/v1/files/tiny-gif/${endpoint}`, async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  }

  await page.goto('/');
  await page.getByLabel('Username').fill('fit-user');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview tiny.gif' }).click();
  const stage = page.locator('.viewer-stage');
  const image = page.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  await stage.focus();
  return image;
}

test('configured non-upscaling default and V cycling apply to animated image viewer content', async ({ page }) => {
  const image = await openTinyAnimatedViewer(page);

  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeGreaterThan(318);
  expect((await image.boundingBox())!.width).toBeLessThan(322);
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).toBe('pixelated');

  await page.keyboard.press('s');
  await expect(page.getByRole('status')).toContainText('Smooth scaling');
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).not.toBe('pixelated');

  await page.keyboard.press('s');
  await expect(page.getByRole('status')).toContainText('Nearest-neighbor scaling');
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).toBe('pixelated');

  await page.keyboard.press('1');
  await expect(page.getByRole('status')).toContainText('Fit window');
  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeGreaterThan(500);

  await page.keyboard.press('v');
  await expect(page.getByRole('status')).toContainText('Fit down only');
  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeLessThan(322);

  await page.keyboard.press('v');
  await expect(page.getByRole('status')).toContainText('Original size if it fits');
  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeLessThan(322);
});
