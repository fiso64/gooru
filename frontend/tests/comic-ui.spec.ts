import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const comic = {
  id: 'comic-1', content_id: 'hash-comic', name: 'issue.cbz', safe_display_path: 'library/issue.cbz', size: 4096,
  modified_time: '2026-05-20T00:00:00Z', media_type: 'application/vnd.comicbook+zip', media_kind: 'document',
  metadata: { image_width: 900, image_height: 1200 }, tags: [],
  media_urls: { thumbnail: '/api/v1/files/comic-1/thumbnail', preview: '/api/v1/files/comic-1/preview', content: '/api/v1/files/comic-1/content', download: '/api/v1/files/comic-1/download' }
};

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ font_style: 'editorial', load_full_media_by_default: false }) }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: loggedIn ? 200 : 401, contentType: 'application/json', body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } }) }));
  await page.route('**/api/v1/auth/login', async (route) => { loggedIn = true; await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }); });
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [comic], total_count: 1, library_count: 1, facets: { kind: [] } }) }));
  await page.route('**/api/v1/comics/comic-1', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ pages: [
    { index: 0, name: '001.jpg', url: '/api/v1/comics/comic-1/0' },
    { index: 1, name: '002.jpg', url: '/api/v1/comics/comic-1/1' },
    { index: 2, name: '003.jpg', url: '/api/v1/comics/comic-1/2' }
  ] }) }));
  await page.route('**/api/v1/files/comic-1/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="1200" />' }));
  await page.route('**/api/v1/files/comic-1/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="1200" />' }));
  await page.route('**/api/v1/comics/comic-1/*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="1200" />' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Preview issue.cbz' }).click();
}

test('comic reader uses concept entry treatment and shared playback controls', async ({ page }) => {
  await mockApp(page);
  const dialog = page.getByRole('dialog', { name: 'issue.cbz' });
  const stage = dialog.locator('.viewer-stage');
  const readComic = dialog.getByRole('button', { name: 'Read comic' });
  await expect(dialog).toBeVisible();
  await expect(readComic).toBeVisible();
  await expect(dialog.locator('.lightbox-aside').getByText('Pages')).toHaveCount(0);
  await expect(dialog.locator('.comic-session')).toHaveCount(0);

  await expect(readComic).toHaveCSS('height', '42px');
  await expect(readComic).toHaveCSS('border-radius', '8px');
  await expect(readComic.locator('.comic-read-arrow-left')).toHaveCSS('background-image', /svg/);
  await expect(readComic.locator('.comic-read-arrow-right')).toHaveCSS('background-image', /svg/);

  const previewImage = stage.locator('img.viewer-visual-media');
  await expect(previewImage).toBeVisible();
  const imageBox = await previewImage.boundingBox();
  const readBox = await readComic.boundingBox();
  if (!imageBox || !readBox) throw new Error('comic preview geometry unavailable');
  expect(Math.abs(readBox.y - (imageBox.y + 12))).toBeLessThanOrEqual(2);
  expect(Math.abs((readBox.x + readBox.width) - (imageBox.x + imageBox.width - 12))).toBeLessThanOrEqual(2);

  await readComic.click();
  await expect(stage).toHaveClass(/comic-reading/);
  await expect.poll(() => stage.evaluate((element) => getComputedStyle(element, '::after').animationName)).toBe('arrow-through');
  const controls = dialog.locator('.lightbox-video-controls.comic-controls');
  await expect(controls).toBeVisible();
  await expect(controls.getByRole('button', { name: 'Exit comic (Space)' })).toBeVisible();
  await expect(controls.locator('.video-time').first()).toHaveText('1');
  await expect(controls.locator('.video-time').last()).toHaveText('3');
  await expect(dialog.getByRole('button', { name: 'Previous page' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Next page' })).toBeVisible();

  await dialog.getByRole('button', { name: 'Next page' }).click();
  await expect(controls.locator('.video-time').first()).toHaveText('2');

  const seek = controls.getByRole('button', { name: 'Seek comic page' });
  await seek.click({ position: { x: 2, y: 2 } });
  await expect(controls.locator('.video-time').first()).toHaveText('1');

  await stage.focus();
  await page.mouse.move(0, 0);
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity), { timeout: 3500 }).toBe('0');
  const box = await stage.boundingBox();
  if (!box) throw new Error('viewer stage has no bounding box');
  await page.mouse.move(box.x + 30, box.y + box.height - 30);
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');

  await stage.focus();
  await page.keyboard.press('Space');
  await expect(dialog.getByRole('button', { name: 'Read comic' })).toBeVisible();
  await expect.poll(() => stage.evaluate((element) => getComputedStyle(element, '::after').animationName)).toBe('arrow-back');
});
