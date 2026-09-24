import { expect, test, type Page } from '@playwright/test';
import { mockFileAround } from './helpers/mockFileAround';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const all = Array.from({ length: 67 }, (_, index) => {
  const id = `file-${String(index).padStart(3, '0')}`;
  return {
    id, content_id: `hash-${id}`, name: `${id}.jpg`,
    safe_display_path: `library/${id}.jpg`, size: 2048,
    modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg',
    metadata: { image_width: 800, image_height: 600 },
    media_kind: 'photo', tags: index % 4 === 0 ? ['flag:yes'] : [],
    media_urls: Object.fromEntries(['thumbnail', 'preview', 'content', 'download'].map((kind) =>
      [kind, `/api/v1/files/${id}/${kind}`]))
  };
});

async function mockApp(page: Page) {
  const matching = (query: string) => query.includes('flag:yes') ? all.filter((item) => item.tags.includes('flag:yes')) : all;
  await page.route('**/api/v1/ui-config', (route) => route.fulfill({ json: { pagination_mode: 'paged', items_per_page: 60 } }));
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ json: session }));
  await page.route('**/api/v1/saved-searches', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/upload-targets', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/tags?**', (route) => route.fulfill({ json: { tags: [] } }));
  await page.route('**/api/v1/search/suggestions?**', (route) => route.fulfill({ json: { items: [] } }));
  await mockFileAround(page, ({ query }) => matching(query));
  await page.route('**/api/v1/files?**', (route) => {
    const params = new URL(route.request().url()).searchParams;
    const files = matching(params.get('query') ?? '');
    const limit = Number(params.get('limit') ?? 60);
    const offset = Number(params.get('offset') ?? 0);
    return route.fulfill({ json: { files: files.slice(offset, offset + limit), total_count: files.length, library_count: all.length, facets: { kind: [] } } });
  });
  await page.route(/\/api\/v1\/files\/[^/]+\/(thumbnail|preview|content)$/, (route) =>
    route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"/>' }));
}

test('previous wraps to the entire listing last file, not page one last file', async ({ page }) => {
  await mockApp(page);
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Preview file-000.jpg' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview file-000.jpg' }).click();
  await page.keyboard.press('ArrowLeft');
  await expect(page.getByRole('dialog', { name: 'file-066.jpg' })).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'file-000.jpg' })).toBeVisible();
});

test('wraparound and neighbors honor the active filter rather than the loaded grid', async ({ page }) => {
  await mockApp(page);
  await page.goto('/?q=flag%3Ayes');
  await expect(page.getByRole('button', { name: 'Preview file-000.jpg' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview file-000.jpg' }).click();
  await page.keyboard.press('ArrowLeft');
  await expect(page.getByRole('dialog', { name: 'file-064.jpg' })).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'file-000.jpg' })).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'file-004.jpg' })).toBeVisible();
});
