import { mockFileAround } from './helpers/mockFileAround';
import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function openVideoViewer(page: Page) {
  let loggedIn = false;
  const file = {
    id: 'clip',
    content_id: 'hash-clip',
    name: 'clip.mp4',
    safe_display_path: 'library/clip.mp4',
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'video/mp4',
    media_kind: 'video',
    metadata: { image_width: 1280, image_height: 720, duration_seconds: 120 },
    tags: [],
    media_urls: {
      thumbnail: '/api/v1/files/clip/thumbnail',
      preview: '/api/v1/files/clip/preview',
      content: '/api/v1/files/clip/content',
      download: '/api/v1/files/clip/download'
    }
  };

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/clip/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"/>'
  }));
  await page.route('**/api/v1/files/clip/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720"/>'
  }));
  await page.route('**/api/v1/files/clip/content', async (route) => route.fulfill({ status: 204, body: '' }));

  await mockFileAround(page, () => [file]);
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Preview clip.mp4' }).click();

  const video = page.locator('video.viewer-visual-media');
  await expect(video).toBeAttached();
  await video.evaluate((node) => {
    const media = node as HTMLVideoElement;
    const state = { time: 30, paused: true };
    Object.defineProperty(media, 'videoWidth', { configurable: true, get: () => 1280 });
    Object.defineProperty(media, 'videoHeight', { configurable: true, get: () => 720 });
    Object.defineProperty(media, 'duration', { configurable: true, get: () => 120 });
    Object.defineProperty(media, 'currentTime', {
      configurable: true,
      get: () => state.time,
      set: (value) => { state.time = Number(value); }
    });
    Object.defineProperty(media, 'paused', { configurable: true, get: () => state.paused });
    media.play = async () => { state.paused = false; media.dispatchEvent(new Event('play')); };
    media.pause = () => { state.paused = true; media.dispatchEvent(new Event('pause')); };
    media.dispatchEvent(new Event('loadedmetadata'));
  });
  await expect(video).toBeVisible();
  return video;
}

test('video scrubbing stays paused and muted until release', async ({ page }) => {
  const video = await openVideoViewer(page);
  const controls = page.locator('.lightbox-video-controls');
  const progress = page.getByRole('button', { name: 'Seek video' });

  const controlsBox = await controls.boundingBox();
  expect(controlsBox).not.toBeNull();
  expect(controlsBox!.width).toBeGreaterThanOrEqual(420);
  expect(controlsBox!.height).toBeGreaterThanOrEqual(38);

  await video.evaluate(async (node) => {
    const media = node as HTMLVideoElement;
    media.muted = false;
    await media.play();
  });
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(false);

  const box = await progress.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width * 0.4, box!.y + box!.height / 2);
  await page.mouse.down();

  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(true);
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).muted)).toBe(true);
  const heldTime = await video.evaluate((node) => (node as HTMLVideoElement).currentTime);
  await page.waitForTimeout(200);
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(true);
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).currentTime)).toBe(heldTime);

  await page.mouse.up();
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(false);
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).muted)).toBe(false);
});
