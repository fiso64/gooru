import { expect, test, type Locator, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const hoverDwellMs = 150;
const controlledClockStart = new Date('2026-01-01T00:00:00Z');
const controlledClockPause = new Date('2026-01-01T00:00:20Z');

// Keep the first red frame visible for three seconds so restart assertions have a wide, deterministic sampling window.
const twoFrameGif = Buffer.from('R0lGODlhAgACAIEAAP8AAAAAAAAAAAAAACH/C05FVFNDQVBFMi4wAwEAAAAh+QQALAEAACwAAAAAAgACAAAIBgABCAQQEAAh+QQBZAABACwAAAAAAgACAIEA/wAAAAAAAAAAAAAIBgABCAQQEAA7', 'base64');
const transparentGif = Buffer.from('R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7', 'base64');

function mediaFile(id: string, kind: 'video' | 'gif') {
  const gif = kind === 'gif';
  const extension = gif ? 'gif' : 'mp4';
  return {
    id,
    content_id: `hash-${id}`,
    name: `${id}.${extension}`,
    safe_display_path: `library/${id}.${extension}`,
    size: 2048,
    modified_time: '2026-09-07T00:00:00Z',
    media_type: gif ? 'image/gif' : 'video/mp4',
    media_kind: kind,
    metadata: gif ? { image_width: 320, image_height: 180 } : { video_width: 320, video_height: 180, duration_seconds: 4 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

function hoverSessionURL(source: string | null) {
  if (!source) throw new Error('missing hover preview source');
  return new URL(source, 'http://gooru.test');
}

async function gifPixel(gif: Locator) {
  return gif.evaluate((element) => {
    const image = element as HTMLImageElement;
    const canvas = document.createElement('canvas');
    canvas.width = Math.max(1, image.naturalWidth);
    canvas.height = Math.max(1, image.naturalHeight);
    const context = canvas.getContext('2d');
    if (!context) throw new Error('missing canvas context');
    context.drawImage(image, 0, 0);
    return Array.from(context.getImageData(0, 0, 1, 1).data);
  });
}

async function mockLibrary(page: Page, uiConfig: Record<string, unknown> = {}) {
  const files = [mediaFile('video-one', 'video'), mediaFile('gif-one', 'gif'), mediaFile('video-two', 'video')];
  let loggedIn = false;

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ hover_play_videos: true, hover_play_gifs: true, grid_size: 200, grid_type: 'square', thumbnail_sizes: [256], ...uiConfig })
  }));
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
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [{ value: 'video', count: 2 }, { value: 'gif', count: 1 }] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail?**', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#fff"/></svg>'
  }));
  await page.route('**/api/v1/files/*/content**', async (route) => route.fulfill({ status: 204 }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function mockLibraryWithHoverClock(page: Page, uiConfig: Record<string, unknown> = {}) {
  await page.clock.install({ time: controlledClockStart });
  await mockLibrary(page, uiConfig);
  await page.clock.pauseAt(controlledClockPause);
}

async function hoverPastDwell(page: Page, target: Locator) {
  await target.hover();
  await page.clock.fastForward(hoverDwellMs);
}

test('video and gif previews start after dwell, stop on leave, and only one is active', async ({ page }) => {
  await mockLibraryWithHoverClock(page);

  const firstVideo = page.getByRole('button', { name: 'Preview video-one.mp4' });
  await firstVideo.hover();
  await page.clock.fastForward(hoverDwellMs - 1);
  await expect(page.getByTestId('hover-video-preview')).toHaveCount(0);
  await page.clock.fastForward(1);
  await expect(page.getByTestId('hover-video-preview')).toHaveCount(1);
  const video = page.getByTestId('hover-video-preview');
  await expect(video).toHaveAttribute('loop', '');
  await expect(video).toHaveAttribute('playsinline', '');
  await expect(page.getByTestId('hover-video-progress')).toHaveCount(1);

  const gif = page.getByRole('button', { name: 'Preview gif-one.gif' });
  await hoverPastDwell(page, gif);
  await expect(page.getByTestId('hover-gif-preview')).toHaveCount(1);
  await expect(page.getByTestId('hover-video-preview')).toHaveCount(0);

  await page.getByRole('heading', { name: 'Library' }).hover();
  await expect(page.getByTestId('hover-gif-preview')).toHaveCount(0);
});

test('hover previews keep the thumbnail visible until media is ready and restart gif sessions', async ({ page }) => {
  await mockLibraryWithHoverClock(page);

  const videoCard = page.getByRole('button', { name: 'Preview video-one.mp4' });
  await hoverPastDwell(page, videoCard);
  const video = page.getByTestId('hover-video-preview');
  await expect(video).toHaveCount(1);
  await expect(video).toHaveCSS('opacity', '0');
  await expect(page.getByTestId('hover-video-progress')).toHaveCSS('opacity', '0');

  const gifCard = page.getByRole('button', { name: 'Preview gif-one.gif' });
  await hoverPastDwell(page, gifCard);
  const firstGif = page.getByTestId('hover-gif-preview');
  await expect(firstGif).toHaveCount(1);
  await expect(firstGif).toHaveCSS('opacity', '0');
  const firstSource = hoverSessionURL(await firstGif.getAttribute('src'));
  expect(firstSource.pathname).toBe('/api/v1/files/gif-one/content');
  expect(firstSource.searchParams.get('gooru_hover_session')).toBe('1');
  expect(firstSource.hash).toBe('#gooru-hover-1');

  await page.getByRole('heading', { name: 'Library' }).hover();
  await expect(page.getByTestId('hover-gif-preview')).toHaveCount(0);
  await hoverPastDwell(page, gifCard);
  const secondGif = page.getByTestId('hover-gif-preview');
  await expect(secondGif).toHaveCount(1);
  const secondSource = hoverSessionURL(await secondGif.getAttribute('src'));
  expect(secondSource.pathname).toBe(firstSource.pathname);
  expect(secondSource.searchParams.get('gooru_hover_session')).toBe('2');
  expect(secondSource.hash).toBe('#gooru-hover-2');
  expect(secondSource.href).not.toBe(firstSource.href);
  expect(`${secondSource.pathname}${secondSource.search}`).not.toBe(`${firstSource.pathname}${firstSource.search}`);
});

test('ready transparent gif replaces the thumbnail backing layer', async ({ page }) => {
  await mockLibraryWithHoverClock(page);
  await page.route('**/api/v1/files/gif-one/content**', async (route) => route.fulfill({ contentType: 'image/gif', body: transparentGif }));

  const gifCard = page.getByRole('button', { name: 'Preview gif-one.gif' });
  const thumbnail = gifCard.locator('img').first();
  await expect(thumbnail).toHaveCSS('opacity', '1');

  await hoverPastDwell(page, gifCard);
  const gif = page.getByTestId('hover-gif-preview');
  await expect(gif).toHaveClass(/is-ready/);
  await expect(gif).toHaveCSS('opacity', '1');
  await expect(thumbnail).toHaveClass(/preview-covered/);
  await expect(thumbnail).toHaveCSS('opacity', '0');

  await page.getByRole('heading', { name: 'Library' }).hover();
  await expect(gif).toHaveCount(0);
  await expect(thumbnail).toHaveCSS('opacity', '1');
});

test('gif playback leaves the normal card affordances above playback', async ({ page }) => {
  await mockLibraryWithHoverClock(page);
  await page.route('**/api/v1/files/gif-one/content**', async (route) => route.fulfill({ contentType: 'image/gif', body: twoFrameGif }));

  const gifCard = page.getByRole('button', { name: 'Preview gif-one.gif' });
  const card = gifCard.locator('..');
  await hoverPastDwell(page, gifCard);
  const gif = page.getByTestId('hover-gif-preview');
  await expect(gif).toHaveClass(/is-ready/);
  await expect(gif).toHaveCSS('z-index', '1');
  await expect(gifCard.locator('.thumb-overlay')).toHaveCSS('z-index', '2');
  await expect(gifCard.locator('.thumb-meta')).toHaveCSS('z-index', '3');
  await expect(gifCard.locator('.thumb-badges')).toHaveCSS('z-index', '3');
  await expect(card.locator('.thumb-checkbox')).toHaveCSS('z-index', '4');
  await expect(card.locator('.thumb-checkbox')).toHaveCSS('opacity', '1');
});

test('reduced motion and disabled media options suppress hover playback', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await mockLibraryWithHoverClock(page, { hover_play_gifs: false });

  await hoverPastDwell(page, page.getByRole('button', { name: 'Preview video-one.mp4' }));
  await expect(page.getByTestId('hover-video-preview')).toHaveCount(0);

  await page.emulateMedia({ reducedMotion: 'no-preference' });
  await hoverPastDwell(page, page.getByRole('button', { name: 'Preview gif-one.gif' }));
  await expect(page.getByTestId('hover-gif-preview')).toHaveCount(0);
});

test('square grid preview keeps the card box geometry stable', async ({ page }) => {
  await mockLibraryWithHoverClock(page);
  const card = page.getByRole('button', { name: 'Preview video-one.mp4' });
  const before = await card.boundingBox();
  await hoverPastDwell(page, card);
  await expect(page.getByTestId('hover-video-preview')).toHaveCount(1);
  const after = await card.boundingBox();
  expect(after).toEqual(before);
});