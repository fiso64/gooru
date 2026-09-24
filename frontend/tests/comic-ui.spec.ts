import { mockFileAround } from './helpers/mockFileAround';
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
  await page.route('**/api/v1/comics/comic-1/*', async (route) => {
    // Make the second page handoff last long enough to exercise the transient freeze canvas.
    // This reproduces the owner's 20–100 ms visual disappearance instead of checking only settled state.
    if (route.request().url().endsWith('/1')) await new Promise((resolve) => setTimeout(resolve, 80));
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="1200" />' });
  });

  await mockFileAround(page, () => [comic]);
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Preview issue.cbz' }).click();
}

test('comic reader matches supplied entry treatment and shared playback controls', async ({ page }) => {
  await mockApp(page);
  const dialog = page.getByRole('dialog', { name: 'issue.cbz' });
  const stage = dialog.locator('.viewer-stage');
  const surface = stage.locator('.viewer-pan-surface');
  const readComic = dialog.getByRole('button', { name: 'Read comic' });
  await expect(dialog).toBeVisible();
  await expect(readComic).toBeVisible();
  await expect(dialog.locator('.lightbox-aside').getByText('Pages')).toHaveCount(0);
  await expect(dialog.locator('.comic-session')).toHaveCount(0);

  // These values and glyph dimensions are copied from the supplied concept's read-cover-button.
  await expect(readComic).toHaveCSS('height', '42px');
  await expect(readComic).toHaveCSS('border-radius', '8px');
  const bookIcon = readComic.locator('svg.icon').first();
  const enterIcon = readComic.locator('svg.enter-arrow');
  await expect(bookIcon).toHaveAttribute('stroke', 'currentColor');
  await expect(bookIcon).toHaveAttribute('stroke-width', '1.55');
  await expect(bookIcon.locator('path').first()).toHaveAttribute('d', 'M4.2 3.8h7.1a2 2 0 0 1 2 2v10.4H6.2a2 2 0 0 1-2-2V3.8Z');
  await expect(enterIcon).toHaveAttribute('stroke', 'currentColor');
  await expect(enterIcon.locator('path')).toHaveAttribute('d', 'M4 10h11M11 6l4 4-4 4');

  const previewImage = stage.locator('img.viewer-visual-media');
  await expect(previewImage).toBeVisible();
  const imageBox = await previewImage.boundingBox();
  const readBox = await readComic.boundingBox();
  if (!imageBox || !readBox) throw new Error('comic preview geometry unavailable');
  expect(Math.abs(readBox.y - (imageBox.y + 12))).toBeLessThanOrEqual(2);
  expect(Math.abs((readBox.x + readBox.width) - (imageBox.x + imageBox.width - 12))).toBeLessThanOrEqual(2);

  await readComic.click();
  await expect(stage).toHaveClass(/comic-reading/);
  await expect(stage).toHaveClass(/entering/);
  await expect(stage.locator('.transition-arrow')).toHaveCount(0);
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-reader-enter');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.48s');
  const controls = dialog.locator('.lightbox-video-controls.comic-controls');
  await expect(controls).toBeVisible();
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-controls-enter');
  await expect.poll(() => stage.evaluate((element) => !element.classList.contains('entering'))).toBe(true);
  await expect(surface).toHaveCSS('transform', 'none');
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).animationName)).toBe('none');
  await expect(controls.getByRole('button', { name: 'Exit comic (Space)' })).toBeVisible();
  await expect(controls.locator('.video-time').first()).toHaveText('1');
  await expect(controls.locator('.video-time').last()).toHaveText('3');
  await expect(dialog.getByRole('button', { name: 'Previous page' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Next page' })).toBeVisible();

  // Page navigation switches the image source immediately. During that handoff the old page is
  // preserved in a z-indexed freeze canvas; the controls must remain in a higher paint layer for
  // the entire load, not merely have opacity 1 again once the next image settles.
  await dialog.getByRole('button', { name: 'Next page' }).click();
  const freeze = stage.locator('.viewer-image-freeze');
  await expect(freeze).toHaveClass(/visible/);
  const paintLayers = await Promise.all([
    controls.evaluate((element) => getComputedStyle(element).zIndex),
    freeze.evaluate((element) => getComputedStyle(element).zIndex)
  ]);
  expect(Number.parseInt(paintLayers[0], 10)).toBeGreaterThan(Number.parseInt(paintLayers[1], 10));
  await expect(controls).toBeVisible();
  await expect(controls).toHaveCSS('opacity', '1');
  await expect(controls.locator('.video-time').first()).toHaveText('2');
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).animationName)).toBe('none');
  await expect.poll(() => freeze.evaluate((element) => !element.classList.contains('visible'))).toBe(true);

  const seek = controls.getByRole('button', { name: 'Seek comic page' });
  await seek.click({ position: { x: 2, y: 2 } });
  await expect(controls.locator('.video-time').first()).toHaveText('1');
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).animationName)).toBe('none');
  await expect(controls).toHaveCSS('opacity', '1');

  await stage.focus();
  await page.mouse.move(0, 0);
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity), { timeout: 3500 }).toBe('0');

  // Page navigation updates progress but is not control-surface activity. Hidden controls must stay hidden.
  await page.keyboard.press('ArrowRight');
  await expect(controls.locator('.video-time').first()).toHaveText('2');
  await page.waitForTimeout(200);
  await expect(controls).toHaveCSS('opacity', '0');

  const box = await stage.boundingBox();
  if (!box) throw new Error('viewer stage has no bounding box');
  await page.mouse.move(box.x + 30, box.y + box.height - 30);
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');

  await stage.focus();
  await page.keyboard.press('Space');
  await expect(dialog.getByRole('button', { name: 'Read comic' })).toBeVisible();
  await expect(stage).toHaveClass(/exiting/);
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationName)).toBe('comic-reader-exit');
  await expect.poll(() => surface.evaluate((element) => getComputedStyle(element).animationDuration)).toBe('0.42s');
  await expect.poll(() => stage.evaluate((element) => !element.classList.contains('exiting'))).toBe(true);
  await expect(surface).toHaveCSS('transform', 'none');
});

test('comic transitions respect reduced-motion preference', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await mockApp(page);
  const dialog = page.getByRole('dialog', { name: 'issue.cbz' });
  const stage = dialog.locator('.viewer-stage');
  await dialog.getByRole('button', { name: 'Read comic' }).click();
  await expect(stage).toHaveClass(/comic-reading/);
  await expect(stage.locator('.viewer-pan-surface')).toHaveCSS('animation-duration', '0.001s');
  await expect(stage.locator('.transition-arrow')).toHaveCount(0);
});
