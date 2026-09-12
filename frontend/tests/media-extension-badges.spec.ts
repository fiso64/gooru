import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: string, name: string, mediaKind: string, mediaType: string, metadata: Record<string, number> = {}) {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: mediaType,
    media_kind: mediaKind,
    metadata,
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  const files = [
    fileItem('photo', 'cover.jpg', 'photo', 'image/jpeg', { image_width: 800, image_height: 600 }),
    fileItem('video', 'clip.mp4', 'video', 'video/mp4', { video_duration: 62 }),
    fileItem('gif', 'loop.gif', 'gif', 'image/gif', { video_duration: 3 }),
    fileItem('audio', 'Track.FlAc', 'audio', 'audio/flac', { audio_duration: 120 }),
    fileItem('other', 'book.cbz', 'other', 'application/vnd.comicbook+zip')
  ];
  let loggedIn = false;

  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: loggedIn ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
    });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], meta_tags: [] })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#fff"/></svg>'
  }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('grid badges reserve extension labels for non-image non-video files and keep GIF neutral', async ({ page }) => {
  await mockApp(page);

  const photo = page.locator('.thumb').filter({ has: page.getByAltText('cover.jpg') });
  const video = page.locator('.thumb').filter({ has: page.getByAltText('clip.mp4') });
  const gif = page.locator('.thumb').filter({ has: page.getByAltText('loop.gif') });
  const audio = page.locator('.thumb').filter({ has: page.getByAltText('Track.FlAc') });
  const other = page.locator('.thumb').filter({ has: page.getByAltText('book.cbz') });

  await expect(photo.locator('.thumb-badge')).toHaveCount(0);
  await expect(video.locator('.thumb-badge')).toContainText('1:02');
  await expect(video.locator('.thumb-badge')).not.toContainText('MP4');
  await expect(gif.locator('.thumb-badge')).toContainText('GIF');
  await expect(gif.locator('.thumb-badge')).not.toHaveClass(/thumb-badge-extension/);
  await expect(audio.locator('.thumb-badge-extension')).toHaveText('FLAC');
  await expect(other.locator('.thumb-badge-extension')).toHaveText('CBZ');

  const accent = await page.locator('.gooru-root').evaluate((node) => getComputedStyle(node).getPropertyValue('--accent').trim());
  const extensionBackground = await audio.locator('.thumb-badge-extension').evaluate((node) => getComputedStyle(node).backgroundColor);
  const gifBackground = await gif.locator('.thumb-badge').evaluate((node) => getComputedStyle(node).backgroundColor);
  const accentProbe = await page.evaluate((value) => {
    const probe = document.createElement('div');
    probe.style.background = value;
    document.body.append(probe);
    const color = getComputedStyle(probe).backgroundColor;
    probe.remove();
    return color;
  }, accent);

  expect(extensionBackground).toBe(accentProbe);
  expect(gifBackground).not.toBe(accentProbe);
});
