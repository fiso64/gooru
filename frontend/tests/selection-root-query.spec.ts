import { expect, test } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const file = {
  id: 'one',
  content_id: 'hash-one',
  name: 'one.jpg',
  safe_display_path: 'library/one.jpg',
  size: 2048,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'image/jpeg',
  media_kind: 'photo',
  metadata: { image_width: 800, image_height: 600 },
  tags: [],
  can_delete: true,
  media_urls: {
    thumbnail: '/api/v1/files/one/thumbnail',
    preview: '/api/v1/files/one/preview',
    content: '/api/v1/files/one/content',
    download: '/api/v1/files/one/download'
  }
};

test('root-library Select all posts the empty query through the real browser path', async ({ page }) => {
  let loggedIn = false;
  let postedQuery: unknown = undefined;
  let downloadSelector: unknown = undefined;
  let downloadCSRF = '';
  let downloadRequested = false;

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
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"/>' }));
  await page.route('**/api/v1/file-selections', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    postedQuery = (route.request().postDataJSON() as { query?: unknown }).query;
    await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'selection-one', count: 1 }) });
  });
  await page.route('**/api/v1/file-selections/*/members', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ file_ids: ['one'] }) }));
  await page.route('**/api/v1/file-downloads', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    downloadSelector = route.request().postDataJSON();
    downloadCSRF = route.request().headers()['x-gooru-csrf'] ?? '';
    await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ id: 'download-one', url: '/api/v1/file-downloads/download-one' }) });
  });
  await page.route('**/api/v1/file-downloads/download-one', async (route) => {
    downloadRequested = true;
    await route.fulfill({
      status: 200,
      contentType: 'application/zip',
      headers: { 'Content-Disposition': 'attachment; filename="gooru-download.zip"' },
      body: 'zip'
    });
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  await page.keyboard.press('a');

  await expect.poll(() => postedQuery).toBe('');
  await expect(page.getByText('1 selected')).toBeVisible();
  await expect(page.getByRole('button', { name: 'Download' })).toBeEnabled();

  await page.keyboard.press('d');

  await expect.poll(() => downloadSelector).toEqual({ selection_id: 'selection-one' });
  expect(downloadCSRF).toBe('csrf-one');
  await expect.poll(() => downloadRequested).toBe(true);
});
