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

function pageOffset(token: string | null) {
  if (!token) return 0;
  if (/^\d+$/.test(token)) return Number(token);
  const normalized = token.replace(/-/g, '+').replace(/_/g, '/');
  const decoded = Buffer.from(normalized, 'base64').toString('utf8');
  const match = /^offset:(\d+)$/.exec(decoded);
  return match ? Number(match[1]) : 0;
}

async function mockPagedLibrary(page: Page) {
  const allFiles = Array.from({ length: 273 }, (_, index) => fileItem(index));
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
    const offset = pageOffset(url.searchParams.get('page_token'));
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

test('paged mode uses centered numbered controls with direct page navigation', async ({ page }) => {
  const requests = await mockPagedLibrary(page);
  await page.goto('/');
  await expect(page.getByText('273 files')).toBeVisible();
  const pager = page.getByTestId('library-pager');
  await expect(pager).toBeVisible();
  await expect(page.getByTestId('infinite-scroll-sentinel')).toHaveCount(0);
  await expect.poll(() => requests.some((request) => request.offset === 0 && request.limit === 25)).toBe(true);

  await expect(page.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('button', { name: 'Page 2', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Page 3', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Page 11', exact: true })).toBeVisible();
  await expect(pager.getByText('…')).toBeVisible();

  await page.getByRole('button', { name: 'Page 3', exact: true }).click();
  await expect.poll(() => requests.some((request) => request.offset === 50 && request.limit === 25)).toBe(true);
  await expect(page.getByRole('button', { name: /page-50\.jpg$/ })).toBeVisible();
  await page.getByRole('button', { name: 'Page 5', exact: true }).click();
  await expect.poll(() => requests.some((request) => request.offset === 100 && request.limit === 25)).toBe(true);

  for (const pageNumber of [1, 3, 4, 5, 6, 7, 11]) {
    await expect(page.getByRole('button', { name: `Page ${pageNumber}`, exact: true })).toBeVisible();
  }
  await expect(page.getByRole('button', { name: 'Page 5', exact: true })).toHaveAttribute('aria-current', 'page');

  const mainBox = await page.locator('main.main').boundingBox();
  const pagerBox = await pager.boundingBox();
  const lastCardBox = await page.getByRole('button', { name: /page-124\.jpg$/ }).boundingBox();
  expect(mainBox).not.toBeNull();
  expect(pagerBox).not.toBeNull();
  expect(lastCardBox).not.toBeNull();
  expect(Math.abs((pagerBox!.x + pagerBox!.width / 2) - (mainBox!.x + mainBox!.width / 2))).toBeLessThan(2);
  expect(pagerBox!.y).toBeGreaterThanOrEqual(lastCardBox!.y + lastCardBox!.height);

  await page.getByRole('button', { name: 'Page 11', exact: true }).click();
  await expect.poll(() => requests.some((request) => request.offset === 250 && request.limit === 25)).toBe(true);
  await expect(page.getByRole('button', { name: /page-250\.jpg$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Page 11', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('button', { name: 'Page 1', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Next page' })).toBeDisabled();
});
