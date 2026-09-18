import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function videoItem() {
  return {
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
}

async function mockApp(page: Page) {
  let loggedIn = false;
  const files = [videoItem()];
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
    body: JSON.stringify({ files, total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/clip/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
  }));
  await page.route('**/api/v1/files/clip/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720"><rect width="1280" height="720"/></svg>'
  }));
  await page.route('**/api/v1/files/clip/content', async (route) => route.fulfill({ status: 204, body: '' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
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
      set: (value) => { state.time = value; }
    });
    Object.defineProperty(media, 'paused', { configurable: true, get: () => state.paused });
    media.play = async () => { state.paused = false; media.dispatchEvent(new Event('play')); };
    media.pause = () => { state.paused = true; media.dispatchEvent(new Event('pause')); };
    media.dispatchEvent(new Event('loadedmetadata'));
  });
  await expect(video).toBeVisible();
  return video;
}

async function controlsOpacity(page: Page) {
  return page.locator('.lightbox-video-controls').evaluate((node) => Number.parseFloat(getComputedStyle(node).opacity));
}

test('keyboard playback shortcuts stay chrome-free and pointer proximity reveals controls', async ({ page }) => {
  const video = await mockApp(page);
  const stage = page.locator('.viewer-stage');
  await page.waitForTimeout(2200);
  await expect.poll(() => controlsOpacity(page)).toBe(0);

  await stage.focus();
  await page.keyboard.press('Shift+ArrowRight');
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).currentTime)).toBe(35);
  await expect.poll(() => controlsOpacity(page)).toBe(0);

  await page.keyboard.press('Space');
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).paused)).toBe(false);
  await expect.poll(() => controlsOpacity(page)).toBe(0);

  const box = await stage.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + 20);
  await page.waitForTimeout(80);
  expect(await controlsOpacity(page)).toBe(0);
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height - 30);
  await expect.poll(() => controlsOpacity(page)).toBeGreaterThan(0.9);
});

test('empty focused tag input keeps shifted arrows for video seeking and shows focus', async ({ page }) => {
  const video = await mockApp(page);
  const input = page.getByRole('textbox', { name: 'Tags for clip.mp4' });
  await input.focus();
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('');

  const editor = page.locator('.lightbox-tag-input');
  const [borderColor, accentColor] = await editor.evaluate((node) => {
    const root = node.closest('.gooru-root') ?? document.documentElement;
    const probe = document.createElement('span');
    probe.style.color = 'var(--accent)';
    root.appendChild(probe);
    const colors = [getComputedStyle(node).borderTopColor, getComputedStyle(probe).color];
    probe.remove();
    return colors;
  });
  expect(borderColor).toBe(accentColor);

  await input.press('Shift+ArrowRight');
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).currentTime)).toBe(35);
  await expect(input).toBeFocused();

  await input.press('Shift+ArrowLeft');
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).currentTime)).toBe(30);
  await expect(input).toBeFocused();
});

test('seekbar drags live and clamps outside both ends', async ({ page }) => {
  const video = await mockApp(page);
  const progress = page.getByRole('button', { name: 'Seek video' });
  const box = await progress.boundingBox();
  expect(box).not.toBeNull();

  await page.mouse.move(box!.x + box!.width * 0.25, box!.y + box!.height / 2);
  await page.mouse.down();
  await page.mouse.move(box!.x + box!.width + 80, box!.y + box!.height / 2, { steps: 4 });
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).currentTime)).toBe(120);
  await page.mouse.move(box!.x - 80, box!.y + box!.height / 2, { steps: 4 });
  await expect.poll(() => video.evaluate((node) => (node as HTMLVideoElement).currentTime)).toBe(0);
  await page.mouse.up();
});

test('fullscreen hides an inactive cursor except over player controls', async ({ page }) => {
  await mockApp(page);
  const stage = page.locator('.viewer-stage');
  await stage.focus();
  await page.keyboard.press('f');
  await expect.poll(() => stage.evaluate((node) => document.fullscreenElement === node)).toBe(true);

  const box = await stage.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.waitForTimeout(1100);
  await expect.poll(() => stage.evaluate((node) => getComputedStyle(node).cursor)).toBe('none');

  const controls = page.locator('.lightbox-video-controls');
  const controlsBox = await controls.boundingBox();
  expect(controlsBox).not.toBeNull();
  await page.mouse.move(controlsBox!.x + controlsBox!.width / 2, controlsBox!.y + controlsBox!.height / 2);
  await page.waitForTimeout(1100);
  expect(await stage.evaluate((node) => getComputedStyle(node).cursor)).not.toBe('none');
});
