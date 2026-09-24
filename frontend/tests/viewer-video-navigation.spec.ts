import { mockFileAround } from './helpers/mockFileAround';
import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: 'image' | 'video') {
  const video = id === 'video';
  return {
    id,
    content_id: `hash-${id}`,
    name: video ? 'clip.mp4' : 'photo.jpg',
    safe_display_path: video ? 'library/clip.mp4' : 'library/photo.jpg',
    size: 4096,
    added_at: '2026-09-05T00:00:00Z',
    modified_time: '2026-09-05T00:00:00Z',
    media_type: video ? 'video/mp4' : 'image/jpeg',
    media_kind: video ? 'video' : 'photo',
    metadata: video ? { duration_seconds: 3 } : { image_width: 900, image_height: 600 },
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
  const files = [fileItem('image'), fileItem('video')];
  let loggedIn = false;
  let pendingVideo: Route | undefined;

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ json: { load_full_media_by_default: false, capabilities: ['preview_images'] } }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [], library_count: files.length }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await mockFileAround(page, () => files);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />'
  }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="600"><rect width="900" height="600" fill="black"/></svg>'
  }));
  await page.route('**/api/v1/files/image/content', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="900" height="600" />'
  }));
  await page.route('**/api/v1/files/video/content', async (route) => {
    pendingVideo = route;
    // Keep the response pending. The regression is that navigation used to wait for a hidden
    // preload request before mounting the actual <video>, so the old image stayed indefinitely.
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  return { pendingVideo: () => pendingVideo };
}

test('image to video navigation mounts the real player without waiting for speculative preload', async ({ page }) => {
  const app = await mockApp(page);
  await page.getByRole('button', { name: 'Preview photo.jpg' }).click();
  await expect(page.getByRole('dialog', { name: 'photo.jpg' })).toBeVisible();
  await expect(page.locator('.viewer-stage img.viewer-visual-media')).toBeVisible();

  await page.keyboard.press('ArrowRight');

  await expect(page.getByRole('dialog', { name: 'clip.mp4' })).toBeVisible();
  const video = page.locator('.viewer-stage video.viewer-visual-media');
  await expect(video).toHaveCount(1);
  await expect(video).toHaveAttribute('src', /\/api\/v1\/files\/video\/content$/);
  await expect.poll(() => Boolean(app.pendingVideo())).toBe(true);
  await expect(page.locator('.viewer-stage img.viewer-visual-media')).toHaveCount(0);

  // A slow real media request may still show a loading state, but it must describe the target
  // clearly rather than the old, ambiguous "Loading latest" wording.
  await page.waitForTimeout(250);
  await expect(page.getByText('Loading latest…')).toHaveCount(0);
  await expect(page.getByText('Loading media…')).toBeVisible();
});
