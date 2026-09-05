import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(index: number) {
  const id = `file-${index}`;
  return {
    id, content_id: `hash-${id}`, name: `page-${index}.jpg`, safe_display_path: `library/${id}.jpg`,
    size: 2048, modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
    metadata: { image_width: 900, image_height: 900 }, tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockPagedLibrary(page: Page) {
  const allFiles = Array.from({ length: 73 }, (_, index) => fileItem(index));
  const requests: Array<{ offset: number; limit: number }> = [];
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ pagination_mode: 'paged', items_per_page: 25, grid_type: 'square', grid_size: 120 }) }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('page_token') ?? '0');
    const limit = Number(url.searchParams.get('limit') ?? '0');
    requests.push({ offset, limit });
    const files = allFiles.slice(offset, offset + limit);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({
      files, total_count: allFiles.length, library_count: allFiles.length,
      facets: offset === 0 ? { kind: [{ value: 'photo', count: allFiles.length }] } : undefined,
      next_page_token: offset + limit < allFiles.length ? String(offset + limit) : undefined,
      previous_page_token: offset > 0 ? String(Math.max(0, offset - limit)) : undefined
    }) });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));
  return requests;
}

test('paged mode keeps one configured page and navigates explicitly', async ({ page }) => {
  const requests = await mockPagedLibrary(page);
  await page.goto('/');
  await expect(page.getByText('73 files')).toBeVisible();
  await expect(page.getByTestId('library-pager')).toContainText('Page 1 of 3');
  await expect(page.getByTestId('infinite-scroll-sentinel')).toHaveCount(0);
  await expect.poll(() => requests.some((request) => request.offset === 0 && request.limit === 25)).toBe(true);
  await expect(page.getByRole('button', { name: /page-0\.jpg$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /page-25\.jpg$/ })).toHaveCount(0);

  const next = page.getByRole('button', { name: 'Next' });
  await next.click();
  await expect(page.getByTestId('library-pager')).toContainText('Page 2 of 3');
  await expect.poll(() => requests.some((request) => request.offset === 25 && request.limit === 25)).toBe(true);
  await expect(page.getByRole('button', { name: /page-25\.jpg$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /page-0\.jpg$/ })).toHaveCount(0);

  await page.getByRole('button', { name: 'Previous' }).click();
  await expect(page.getByTestId('library-pager')).toContainText('Page 1 of 3');
  await expect(page.getByRole('button', { name: /page-0\.jpg$/ })).toBeVisible();
});
