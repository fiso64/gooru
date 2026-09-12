import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};
const aspects = [[1600, 900], [600, 900], [900, 900], [900, 1400], [1200, 800], [700, 1200], [2000, 800]] as const;
type GridType = 'square' | 'fit' | 'tile';
type FileRequest = { offset: number; limit: number; includeFacets: boolean };

function fileItem(index: number) {
  const id = `file-${index}`;
  const [width, height] = aspects[index % aspects.length];
  return {
    id,
    content_id: `hash-${id}`,
    name: `page-${index}.jpg`,
    safe_display_path: `library/${id}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: width, image_height: height },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockLibrary(page: Page, paginationMode: 'infinite' | 'paged', gridType: GridType = 'square') {
  const allFiles = Array.from({ length: 180 }, (_, index) => fileItem(index));
  const requests: FileRequest[] = [];
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ grid_type: gridType, grid_size: 200, pagination_mode: paginationMode, items_per_page: 60 })
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [], library_count: allFiles.length, facets: { kind: [{ value: 'photo', count: allFiles.length }] } }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const raw = url.searchParams.get('page_token') ?? '';
    let offset = Number(raw || 0);
    if (raw && !Number.isFinite(offset)) {
      const decoded = Buffer.from(raw.replace(/-/g, '+').replace(/_/g, '/'), 'base64').toString('utf8');
      offset = Number(decoded.replace(/^offset:/, ''));
    }
    const limit = Number(url.searchParams.get('limit') ?? 60);
    const includeFacets = url.searchParams.get('include_facets') === 'true';
    requests.push({ offset, limit, includeFacets });
    const files = allFiles.slice(offset, offset + limit);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({
      files,
      total_count: allFiles.length,
      library_count: allFiles.length,
      facets: includeFacets ? { kind: [{ value: 'photo', count: allFiles.length }] } : undefined,
      next_page_token: offset + limit < allFiles.length ? String(offset + limit) : undefined,
      previous_page_token: offset > 0 ? String(Math.max(0, offset - limit)) : undefined
    }) });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  return { requests };
}

async function intersectsScrollViewport(page: Page, accessibleName: string) {
  const card = page.getByRole('button', { name: accessibleName });
  if (await card.count() === 0) return false;
  const [cardBox, mainBox] = await Promise.all([card.boundingBox(), page.locator('.main').boundingBox()]);
  if (!cardBox || !mainBox) return false;
  return cardBox.y + cardBox.height > mainBox.y && cardBox.y < mainBox.y + mainBox.height;
}

for (const gridType of ['square', 'fit', 'tile'] as const) {
  test(`${gridType} paged mode renders partial final rows and reuses page aggregates`, async ({ page }) => {
    // 1800px makes both square and fit layouts end a 60-item page on a partial row,
    // avoiding the false confidence of a page size that happens to divide the column count.
    await page.setViewportSize({ width: 1800, height: 820 });
    const library = await mockLibrary(page, 'paged', gridType);
    await page.goto('/');
    await expect(page.getByText('180 files')).toBeVisible();
    const main = page.locator('.main');
    await main.evaluate((node) => { node.scrollTop = node.scrollHeight; node.dispatchEvent(new Event('scroll')); });
    await expect(page.getByRole('button', { name: 'Preview page-59.jpg' })).toBeVisible();
    expect(library.requests.some((request) => request.limit === 1)).toBe(false);
    expect(library.requests.filter((request) => request.limit === 60).every((request) => request.includeFacets)).toBe(true);

    await page.getByTestId('library-pager').getByRole('button', { name: 'Page 2' }).click();
    await expect(page.getByRole('button', { name: 'Preview page-60.jpg' })).toBeVisible();
    await main.evaluate((node) => { node.scrollTop = node.scrollHeight; node.dispatchEvent(new Event('scroll')); });
    await expect(page.getByRole('button', { name: 'Preview page-119.jpg' })).toBeVisible();
    expect(library.requests.some((request) => request.limit === 1)).toBe(false);
  });
}

for (const gridType of ['square', 'fit', 'tile'] as const) {
  test(`${gridType} infinite mode requests page 2 before the retained tail is exhausted`, async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 820 });
    const library = await mockLibrary(page, 'infinite', gridType);
    await page.goto('/');
    await expect(page.getByText('180 files')).toBeVisible();
    const main = page.locator('.main');
    // Different grid modes have different row heights/column counts. Scroll until transport
    // prefetch fires rather than imposing an arbitrary pixel budget, then assert the trigger
    // still happened before the last card of the retained page entered the scroll viewport.
    for (let i = 0; i < 50 && !library.requests.some((request) => request.offset === 60); i += 1) {
      await main.evaluate((node) => { node.scrollTop += 120; node.dispatchEvent(new Event('scroll')); });
      await page.waitForTimeout(20);
    }
    await expect.poll(() => library.requests.some((request) => request.offset === 60)).toBe(true);
    expect(await intersectsScrollViewport(page, 'Preview page-59.jpg')).toBe(false);
  });
}
