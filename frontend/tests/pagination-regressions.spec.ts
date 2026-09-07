import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(index: number) {
  const id = `file-${index}`;
  return {
    id,
    content_id: `hash-${id}`,
    name: `page-${index}.jpg`,
    safe_display_path: `library/${id}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 1200, image_height: 800 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockLibrary(page: Page, paginationMode: 'infinite' | 'paged') {
  const allFiles = Array.from({ length: 180 }, (_, index) => fileItem(index));
  const requests: number[] = [];
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ grid_type: 'square', grid_size: 200, pagination_mode: paginationMode, items_per_page: 60 })
  }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
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
    requests.push(offset);
    const files = allFiles.slice(offset, offset + 60);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({
      files,
      total_count: allFiles.length,
      library_count: allFiles.length,
      facets: { kind: [{ value: 'photo', count: allFiles.length }] },
      next_page_token: offset + 60 < allFiles.length ? String(offset + 60) : undefined,
      previous_page_token: offset > 0 ? String(Math.max(0, offset - 60)) : undefined
    }) });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  return { requests };
}

test('paged mode renders the final row of every 60-item page', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 820 });
  await mockLibrary(page, 'paged');
  await page.goto('/');
  await expect(page.getByText('180 files')).toBeVisible();
  const main = page.locator('.main');
  await main.evaluate((node) => { node.scrollTop = node.scrollHeight; node.dispatchEvent(new Event('scroll')); });
  await expect(page.getByRole('button', { name: 'Open page-59.jpg' })).toBeVisible();

  await page.getByTestId('library-pager').getByRole('button', { name: 'Page 2' }).click();
  await expect(page.getByRole('button', { name: 'Open page-60.jpg' })).toBeVisible();
  await main.evaluate((node) => { node.scrollTop = node.scrollHeight; node.dispatchEvent(new Event('scroll')); });
  await expect(page.getByRole('button', { name: 'Open page-119.jpg' })).toBeVisible();
});

test('infinite mode requests page 2 before the retained tail reaches mid-viewport', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 820 });
  const library = await mockLibrary(page, 'infinite');
  await page.goto('/');
  await expect(page.getByText('180 files')).toBeVisible();
  const main = page.locator('.main');
  const grid = page.locator('.virtual-grid');
  for (let i = 0; i < 20 && !library.requests.includes(60); i += 1) {
    const metrics = await Promise.all([
      main.evaluate((node) => ({ top: node.scrollTop, height: node.clientHeight })),
      grid.evaluate((node) => ({ top: (node as HTMLElement).offsetTop, height: (node as HTMLElement).offsetHeight }))
    ]);
    const viewport = metrics[0];
    const retainedBottom = metrics[1].top + metrics[1].height;
    expect(retainedBottom - viewport.top).toBeGreaterThan(viewport.height / 2);
    await main.evaluate((node) => { node.scrollTop += 120; node.dispatchEvent(new Event('scroll')); });
    await page.waitForTimeout(20);
  }
  await expect.poll(() => library.requests.includes(60)).toBe(true);
});
