import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_fit', username: 'fit-user', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-fit'
};

async function mockSession(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401, contentType: 'application/json',
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
}

async function signInAndOpen(page: Page, filename: string) {
  await page.goto('/');
  await page.getByLabel('Username').fill('fit-user');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await page.getByRole('button', { name: `Preview ${filename}` }).click();
}

async function openTinyAnimatedViewer(page: Page) {
  await mockSession(page);
  const file = {
    id: 'tiny-gif', content_id: 'hash-tiny-gif', name: 'tiny.gif', safe_display_path: 'library/tiny.gif', size: 1024,
    added_at: '2026-09-01T00:00:00Z', modified_time: '2026-09-01T00:00:00Z', media_type: 'image/gif', media_kind: 'gif',
    metadata: { image_width: 320, image_height: 200 }, tags: [], can_delete: true,
    media_urls: {
      thumbnail: '/api/v1/files/tiny-gif/thumbnail', preview: '/api/v1/files/tiny-gif/preview',
      content: '/api/v1/files/tiny-gif/content', download: '/api/v1/files/tiny-gif/download'
    }
  };

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ viewer_fit_mode: 'original_size_if_fit', viewer_scaling: 'nearest' })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="200"><rect width="320" height="200"/></svg>';
  for (const endpoint of ['thumbnail', 'preview', 'content']) {
    await page.route(`**/api/v1/files/tiny-gif/${endpoint}`, async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  }

  await signInAndOpen(page, 'tiny.gif');
  const stage = page.locator('.viewer-stage');
  const image = page.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  await stage.focus();
  return image;
}

async function openComicViewer(page: Page) {
  await mockSession(page);
  const comic = {
    id: 'comic', content_id: 'hash-comic', name: 'issue.cbz', safe_display_path: 'library/issue.cbz', size: 4096,
    added_at: '2026-09-01T00:00:00Z', modified_time: '2026-09-01T00:00:00Z', media_type: 'application/vnd.comicbook+zip', media_kind: 'document',
    metadata: { image_width: 600, image_height: 900 }, tags: [], can_delete: true,
    media_urls: {
      thumbnail: '/api/v1/files/comic/thumbnail', preview: '/api/v1/files/comic/preview',
      content: '/api/v1/files/comic/content', download: '/api/v1/files/comic/download'
    }
  };
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ viewer_scaling: 'nearest' })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ files: [comic], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/comics/comic', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ pages: [
      { index: 0, name: '001.png', url: '/api/v1/comics/comic/0' },
      { index: 1, name: '002.png', url: '/api/v1/comics/comic/1' }
    ] })
  }));
  const cover = '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="900"><rect width="600" height="900"/></svg>';
  await page.route('**/api/v1/files/comic/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: cover }));
  await page.route('**/api/v1/files/comic/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: cover }));
  await page.route('**/api/v1/comics/comic/*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: cover }));

  await signInAndOpen(page, 'issue.cbz');
  const dialog = page.getByRole('dialog', { name: 'issue.cbz' });
  const stage = dialog.locator('.viewer-stage');
  const image = stage.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  await stage.focus();
  return { dialog, stage, image };
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

test('nearest scaling applies to comic pages and follows the shared S toggle', async ({ page }) => {
  const { dialog, stage, image } = await openComicViewer(page);
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).toBe('pixelated');

  await dialog.getByRole('button', { name: 'Read comic' }).click();
  await expect(stage).toHaveClass(/comic-reading/);
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).toBe('pixelated');

  await dialog.getByRole('button', { name: 'Next page' }).click();
  await expect.poll(async () => image.getAttribute('src')).toContain('/api/v1/comics/comic/1');
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).toBe('pixelated');

  await stage.focus();
  await page.keyboard.press('s');
  await expect(page.getByRole('status')).toContainText('Smooth scaling');
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).imageRendering)).not.toBe('pixelated');
});


async function openOversizedActualViewer(page: Page, fitCap?: boolean) {
  await mockSession(page);
  const file = {
    id: 'large-image', content_id: 'hash-large-image', name: 'large.png', safe_display_path: 'library/large.png', size: 4096,
    added_at: '2026-09-07T00:00:00Z', modified_time: '2026-09-07T00:00:00Z', media_type: 'image/png', media_kind: 'photo',
    metadata: { image_width: 2400, image_height: 1600 }, tags: [], can_delete: true,
    media_urls: {
      thumbnail: '/api/v1/files/large-image/thumbnail', preview: '/api/v1/files/large-image/preview',
      content: '/api/v1/files/large-image/content', download: '/api/v1/files/large-image/download'
    }
  };
  const uiConfig: Record<string, unknown> = { viewer_fit_mode: 'actual' };
  if (fitCap !== undefined) uiConfig.viewer_actual_size_fit_cap = fitCap;
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(uiConfig) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="2400" height="1600"><rect width="2400" height="1600"/></svg>';
  for (const endpoint of ['thumbnail', 'preview', 'content']) {
    await page.route(`**/api/v1/files/large-image/${endpoint}`, async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  }
  await signInAndOpen(page, 'large.png');
  const stage = page.locator('.viewer-stage');
  const image = page.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  await stage.focus();
  return { stage, image };
}

test('actual-size mode is fit-capped by default while manual zoom can grow beyond the fitted baseline', async ({ page }) => {
  const { stage, image } = await openOversizedActualViewer(page);
  const stageBox = await stage.boundingBox();
  const initial = await image.boundingBox();
  expect(stageBox).not.toBeNull();
  expect(initial).not.toBeNull();
  expect(initial!.width).toBeLessThanOrEqual(stageBox!.width);
  expect(initial!.height).toBeLessThanOrEqual(stageBox!.height);
  expect(initial!.width).toBeLessThan(2400);

  await page.locator('.viewer-pan-viewport').dispatchEvent('wheel', { deltaY: -120, ctrlKey: true, clientX: stageBox!.x + stageBox!.width / 2, clientY: stageBox!.y + stageBox!.height / 2 });
  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeGreaterThan(initial!.width);
});

test('actual-size fit cap can be disabled to preserve intrinsic 1:1 startup size', async ({ page }) => {
  const { image } = await openOversizedActualViewer(page, false);
  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeGreaterThan(2398);
  expect((await image.boundingBox())!.width).toBeLessThan(2402);
});
