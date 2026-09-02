import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const comic = {
  id: 'comic',
  content_id: 'hash-comic',
  name: 'book.cbz',
  safe_display_path: 'library/book.cbz',
  size: 90 * 1024 * 1024,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'application/vnd.comicbook+zip',
  media_kind: 'other',
  metadata: { image_width: 1200, image_height: 1800 },
  tags: [],
  media_urls: {
    thumbnail: '/api/v1/files/comic/thumbnail',
    preview: '/api/v1/files/comic/preview',
    content: '/api/v1/files/comic/content',
    download: '/api/v1/files/comic/download'
  }
};

async function mockComicApp(page: Page) {
  let loggedIn = false;
  const requestedPages: number[] = [];
  const pages = Array.from({ length: 8 }, (_, index) => ({
    index,
    name: `${String(index + 1).padStart(3, '0')}.jpg`,
    url: `/api/v1/comics/comic/${index}`
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
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [comic], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/comic/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="80" height="120"><rect width="80" height="120"/></svg>'
  }));
  await page.route('**/api/v1/files/comic/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="1800"><rect width="1200" height="1800"/></svg>'
  }));
  await page.route('**/api/v1/files/comic/content', async (route) => route.fulfill({ status: 404, body: '' }));
  await page.route('**/api/v1/comics/comic', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ pages })
  }));
  await page.route('**/api/v1/comics/comic/*', async (route) => {
    const index = Number(new URL(route.request().url()).pathname.split('/').at(-1));
    requestedPages.push(index);
    await new Promise((resolve) => setTimeout(resolve, 20));
    await route.fulfill({
      contentType: 'image/svg+xml',
      body: `<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="1800"><rect width="1200" height="1800"/></svg>`
    });
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  return requestedPages;
}

test('comic navigation shares decode-free speculative image warming', async ({ page }) => {
  await page.addInitScript(() => {
    const originalDecode = HTMLImageElement.prototype.decode;
    const state = window as typeof window & { __comicDecodeCalls?: string[] };
    state.__comicDecodeCalls = [];
    HTMLImageElement.prototype.decode = function () {
      if (this.src.includes('/api/v1/comics/comic/')) {
        state.__comicDecodeCalls?.push(this.src);
        return new Promise<void>(() => {});
      }
      return originalDecode ? originalDecode.call(this) : Promise.resolve();
    };
  });

  const requestedPages = await mockComicApp(page);
  await page.getByRole('button', { name: 'Preview book.cbz' }).click();
  await page.getByRole('button', { name: 'Read comic' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toHaveAttribute('src', /\/api\/v1\/comics\/comic\/0$/);
  await expect.poll(() => requestedPages.includes(1)).toBe(true);
  expect(await page.evaluate(() => (window as typeof window & { __comicDecodeCalls?: string[] }).__comicDecodeCalls ?? [])).toEqual([]);

  const stage = page.locator('.viewer-stage');
  await stage.focus();
  for (let index = 0; index < 5; index += 1) await page.keyboard.press('ArrowRight');
  await expect(page.getByText('Page 6 / 8')).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/api\/v1\/comics\/comic\/5$/);
  await expect.poll(() => requestedPages.includes(6)).toBe(true);
  expect(await page.evaluate(() => (window as typeof window & { __comicDecodeCalls?: string[] }).__comicDecodeCalls ?? [])).toEqual([]);

  for (let index = 0; index < 3; index += 1) await page.keyboard.press('ArrowLeft');
  await expect(page.getByText('Page 3 / 8')).toBeVisible();
  await expect(media).toHaveAttribute('src', /\/api\/v1\/comics\/comic\/2$/);
  expect(await page.evaluate(() => (window as typeof window & { __comicDecodeCalls?: string[] }).__comicDecodeCalls ?? [])).toEqual([]);
});
