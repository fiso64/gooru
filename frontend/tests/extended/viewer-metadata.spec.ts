import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: string, name: string, mediaType: string, size: number, metadata: Record<string, number> = {}) {
  return {
    id, content_id: `hash-${id}`, name, safe_display_path: `uploads/${name}`, size,
    added_at: '2026-09-03T11:22:00Z', modified_time: '2026-09-02T15:08:00Z', media_type: mediaType, media_kind: 'other', metadata, tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`
    }, can_delete: false
  };
}

async function mockApp(page: Page) {
  const files = [
    fileItem('text', 'CONFIG.md', 'text/markdown; charset=utf-8', 7680),
    fileItem('comic', 'book.cbz', 'application/vnd.comicbook+zip', 2048, { page_count: 12 })
  ];
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401, contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  for (const path of ['jobs', 'saved-searches']) {
    await page.route(`**/api/v1/${path}`, async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  }
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: 2, library_count: 2, facets: { kind: [] } }) }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="64" />' }));
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function openViewerFor(page: Page, name: string) {
  const card = page.locator('.thumb').filter({ has: page.getByAltText(name) });
  await card.getByRole('button', { name: `Preview ${name}` }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
}

test('viewer size omits duplicate MIME for generic files and uses persisted CBZ page count', async ({ page }) => {
  await mockApp(page);
  await openViewerFor(page, 'CONFIG.md');
  const genericMeta = page.locator('.lightbox-meta');
  await expect(genericMeta.locator('dd').nth(1)).toHaveText('7.5 KB');
  await expect(genericMeta).toContainText('text/markdown; charset=utf-8');
  await expect(genericMeta.locator('dt')).toHaveText(['Path', 'Size', 'Added', 'Modified', 'Mime', 'Fingerprint']);
  await expect(genericMeta.locator('dd').nth(2)).toContainText('03 Sept 2026');
  await expect(genericMeta.locator('dd').nth(3)).toContainText('02 Sept 2026');
  await expect(genericMeta.locator('dd').nth(5)).toHaveText('hash-text');
  await page.keyboard.press('Escape');

  await openViewerFor(page, 'book.cbz');
  const comicMeta = page.locator('.lightbox-meta');
  await expect(comicMeta.locator('dd').nth(1)).toHaveText('12 pages · 2.0 KB');
  await expect(comicMeta.getByText('Pages', { exact: true })).toHaveCount(0);
});
