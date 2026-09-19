import { expect, test, type Page } from '@playwright/test';

type Kind = 'comic' | 'video';
const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function openViewer(page: Page, kind: Kind) {
  let loggedIn = false;
  const comic = kind === 'comic';
  const name = comic ? 'issue.cbz' : 'clip.mp4';
  const file = {
    id: 'one', content_id: 'hash-one', name, safe_display_path: `library/${name}`, size: 4096,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: comic ? 'application/vnd.comicbook+zip' : 'video/mp4',
    media_kind: comic ? 'document' : 'video',
    metadata: { image_width: 800, image_height: 600, duration_seconds: 120 },
    tags: [], media_urls: {
      thumbnail: '/api/v1/files/one/thumbnail',
      preview: '/api/v1/files/one/preview',
      content: '/api/v1/files/one/content',
      download: '/api/v1/files/one/download'
    }
  };
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401, contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  for (const path of ['saved-searches', 'upload-targets']) {
    await page.route(`**/api/v1/${path}`, async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  }
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: '{"tags":[]}' }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="60"/>'
  }));
  await page.route('**/api/v1/files/one/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600"/>'
  }));
  if (comic) {
    await page.route('**/api/v1/comics/one', async (route) => route.fulfill({
      contentType: 'application/json', body: JSON.stringify({ pages: [{ index: 0, name: '001.jpg', url: '/api/v1/comics/one/0' }] })
    }));
    await page.route('**/api/v1/comics/one/*', async (route) => route.fulfill({
      contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600"/>'
    }));
  } else {
    await page.route('**/api/v1/files/one/content', async (route) => route.fulfill({ status: 204, body: '' }));
  }
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: `Preview ${name}` }).click();
  return page.getByRole('textbox', { name: `Tags for ${name}` });
}

test('empty comic tag input delegates Space to enter and exit reader', async ({ page }) => {
  const input = await openViewer(page, 'comic');
  const dialog = page.getByRole('dialog', { name: 'issue.cbz' });
  const stage = dialog.locator('.viewer-stage');
  await expect(dialog.getByRole('button', { name: 'Read comic' })).toBeVisible();
  await input.focus();
  await input.press('Space');
  await expect(input).toHaveValue('');
  await expect(stage).toHaveClass(/comic-reading/);
  await expect.poll(() => stage.evaluate((node) => !node.classList.contains('entering'))).toBe(true);
  await input.focus();
  await input.press('Space');
  await expect(input).toHaveValue('');
  await expect(dialog.getByRole('button', { name: 'Read comic' })).toBeVisible();
});

test('empty video tag input delegates Space to play and pause', async ({ page }) => {
  const input = await openViewer(page, 'video');
  const video = page.locator('video.viewer-visual-media');
  await expect(video).toBeAttached();
  await video.evaluate((node) => {
    const media = node as HTMLVideoElement;
    const state = { paused: true };
    Object.defineProperty(media, 'paused', { configurable: true, get: () => state.paused });
    media.play = async () => { state.paused = false; media.dispatchEvent(new Event('play')); };
    media.pause = () => { state.paused = true; media.dispatchEvent(new Event('pause')); };
  });
  await input.focus();
  await input.press('Space');
  await expect(input).toHaveValue('');
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(false);
  await input.focus();
  await input.press('Space');
  await expect(input).toHaveValue('');
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(true);
});
