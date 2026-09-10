import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type MediaKind = 'video' | 'audio';

function fileItem(kind: MediaKind) {
  const id = kind;
  const name = kind === 'video' ? 'clip.mp4' : 'song.mp3';
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: 1024,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: kind === 'video' ? 'video/mp4' : 'audio/mpeg',
    media_kind: kind,
    metadata: kind === 'video'
      ? { video_width: 1920, video_height: 1080, video_duration: 20 }
      : { audio_duration: 20 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page, kind: MediaKind) {
  let loggedIn = false;
  const files = [fileItem(kind)];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
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
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" />' }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" />' }));
  await page.route('**/api/v1/files/*/content', async (route) => route.fulfill({ status: 404, contentType: 'text/plain', body: 'fixture unavailable' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: `Preview ${files[0].name}` }).click();
  await expect(page.getByRole('dialog', { name: files[0].name })).toBeVisible();
}

async function makeMediaControllable(page: Page, selector: 'video' | 'audio') {
  const media = page.locator(selector);
  await expect(media).toHaveCount(1);
  await media.evaluate((node) => {
    const element = node as HTMLMediaElement;
    let currentTime = 10;
    Object.defineProperty(element, 'paused', { configurable: true, get: () => false });
    Object.defineProperty(element, 'duration', { configurable: true, get: () => 20 });
    Object.defineProperty(element, 'currentTime', {
      configurable: true,
      get: () => currentTime,
      set: (value) => { currentTime = Number(value); }
    });
    (window as typeof window & { __seekTime?: () => number }).__seekTime = () => currentTime;
    element.dispatchEvent(new Event('loadedmetadata'));
  });
}

async function currentTime(page: Page) {
  return page.evaluate(() => (window as typeof window & { __seekTime?: () => number }).__seekTime?.());
}

for (const kind of ['video', 'audio'] as const) {
  test(`Shift+Left/Right seeks ${kind} by five seconds`, async ({ page }) => {
    await mockApp(page, kind);
    await makeMediaControllable(page, kind);
    await page.locator('.viewer-stage').focus();

    await page.keyboard.press('Shift+ArrowRight');
    await expect.poll(() => currentTime(page)).toBe(15);

    await page.keyboard.press('Shift+ArrowLeft');
    await expect.poll(() => currentTime(page)).toBe(10);
  });
}

test('complete video control panel fades after idle and returns on interaction', async ({ page }) => {
  await mockApp(page, 'video');
  await makeMediaControllable(page, 'video');
  const stage = page.locator('.viewer-stage');
  const controls = page.locator('.lightbox-video-controls');
  const seekbar = page.getByRole('button', { name: 'Seek video' });

  await expect(controls.getByRole('button', { name: 'Pause video' })).toBeVisible();
  await expect(controls.locator('.video-time')).toHaveCount(2);
  await expect(seekbar).toBeVisible();
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');
  await expect.poll(() => seekbar.evaluate((element) => getComputedStyle(element, '::before').opacity)).toBe('1');

  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity), { timeout: 3500 }).toBe('0');

  const box = await stage.boundingBox();
  if (!box) throw new Error('viewer stage has no bounding box');
  // Player chrome is proximity-triggered: movement near the lower control zone reveals it.
  await page.mouse.move(box.x + 30, box.y + box.height - 30);
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');

  await seekbar.focus();
  await page.waitForTimeout(2200);
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');

  await stage.focus();
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity), { timeout: 3500 }).toBe('0');

  await page.keyboard.press('Shift+ArrowRight');
  // Keyboard seek intentionally stays chrome-free; #184 made player chrome proximity/focus driven.
  await expect.poll(() => controls.evaluate((element) => getComputedStyle(element).opacity)).toBe('0');
});
