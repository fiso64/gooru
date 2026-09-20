import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const files = ['one', 'two'].map((id) => ({
  id, content_id: 'hash-' + id, name: id + '.jpg', safe_display_path: 'library/' + id + '.jpg',
  size: 2048, modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg',
  media_kind: 'photo', metadata: { image_width: 800, image_height: 600 }, tags: [], can_delete: true,
  media_urls: Object.fromEntries(['thumbnail', 'preview', 'content', 'download'].map((type) =>
    [type, '/api/v1/files/' + id + '/' + type]))
}));

async function mockApp(page: Page) {
  await page.route('**/api/v1/ui-config', (route) =>
    route.fulfill({ json: { fullscreen_media_by_default: false } }));
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ json: session }));
  await page.route('**/api/v1/saved-searches', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/upload-targets', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/tags?**', (route) => route.fulfill({ json: { tags: [] } }));
  await page.route('**/api/v1/search/suggestions?**', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/files?**', (route) => route.fulfill({
    json: { files, total_count: 2, library_count: 2, facets: { kind: [] } }
  }));
  await page.route(/\/api\/v1\/files\/(one|two)\/(thumbnail|preview|content)(\?.*)?$/, (route) =>
    route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"/>' }));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
}

test('viewer action dialog blocks background shortcuts and fullscreen renders overlays', async ({ page }) => {
  await mockApp(page);
  await page.keyboard.press('f');
  await expect.poll(() => page.evaluate(() => document.fullscreenElement?.classList.contains('viewer-stage') ?? false)).toBe(true);
  await page.keyboard.press('Delete');
  const action = page.getByRole('dialog', { name: 'Remove from library' });
  await expect(action).toBeVisible();
  await expect(page.locator('.viewer-stage .modal-backdrop')).toBeVisible();
  await page.keyboard.press('j');
  await page.keyboard.press('t');
  await page.keyboard.press('?');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
  await expect(page.getByRole('dialog', { name: 'Shortcuts' })).toHaveCount(0);
  await expect(page.getByRole('textbox', { name: 'Tags for one.jpg' })).not.toBeFocused();
  await page.keyboard.press('Escape');
  await expect(action).toHaveCount(0);
  if (await page.evaluate(() => document.fullscreenElement === null)) {
    await page.getByRole('dialog', { name: 'one.jpg' }).focus();
    await page.keyboard.press('f');
  }
  await expect.poll(() => page.evaluate(() => document.fullscreenElement?.classList.contains('viewer-stage') ?? false)).toBe(true);
  await page.keyboard.press('?');
  const shortcuts = page.getByRole('dialog', { name: 'Shortcuts' });
  await expect(shortcuts).toBeVisible();
  await expect(page.locator('.viewer-stage .shortcut-backdrop')).toBeVisible();
  await page.keyboard.press('k');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(shortcuts).toHaveCount(0);
});
