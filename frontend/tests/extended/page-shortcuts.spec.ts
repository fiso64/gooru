import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockSignedInShell(page: Page, uiConfig: Record<string, unknown> = {}) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(uiConfig) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

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
    metadata: { image_width: 900, image_height: 900 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
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

test('Shift+PageUp/PageDown navigates paged library and leaves text editing alone', async ({ page }) => {
  const allFiles = Array.from({ length: 6 }, (_, index) => fileItem(index));
  const offsets: number[] = [];
  await mockSignedInShell(page, { pagination_mode: 'paged', items_per_page: 2, grid_type: 'square', grid_size: 120 });
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const offset = pageOffset(url.searchParams.get('page_token'));
    const limit = Number(url.searchParams.get('limit') ?? '2');
    offsets.push(offset);
    const files = allFiles.slice(offset, offset + limit);
    const hasNext = offset + limit < allFiles.length;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files,
        total_count: allFiles.length,
        library_count: allFiles.length,
        facets: { kind: [{ value: 'photo', count: allFiles.length }] },
        next_page_token: hasNext ? String(offset + limit) : undefined,
        previous_page_token: offset > 0 ? String(Math.max(0, offset - limit)) : undefined
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
  }));

  await page.goto('/');
  const pager = page.getByTestId('library-pager');
  await expect(pager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');

  await page.keyboard.press('Shift+PageDown');
  await expect(pager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect.poll(() => offsets.includes(2)).toBe(true);

  await page.keyboard.press('Shift+PageUp');
  await expect(pager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');

  const search = page.getByRole('textbox', { name: 'Search library' });
  await search.focus();
  await page.keyboard.press('Shift+PageDown');
  await expect(pager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');
});

function operation(index: number) {
  return {
    id: `operation-${index}`,
    kind: `history_${index}`,
    status: index < 7 ? 'running' : 'completed',
    progress_total: 10,
    progress_completed: index < 7 ? 5 : 10,
    progress_failed: 0,
    created_at: new Date(Date.UTC(2026, 8, 5, 12, 0, index)).toISOString(),
    finished_at: index < 7 ? undefined : new Date(Date.UTC(2026, 8, 5, 12, 0, index + 1)).toISOString()
  };
}

test('Jobs duplicate pagers handle one Shift+PageDown action per keypress', async ({ page }) => {
  const operations = Array.from({ length: 120 }, (_, index) => operation(index)).reverse();
  const offsets: number[] = [];
  await mockSignedInShell(page);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/operations?**', async (route) => {
    const url = new URL(route.request().url());
    const limit = Number(url.searchParams.get('limit') ?? '50');
    const offset = Number(url.searchParams.get('offset') ?? '0');
    offsets.push(offset);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: operations.slice(offset, offset + limit), active_count: 7, total_count: operations.length })
    });
  });

  await page.goto('/');
  await page.locator('.topbar-right').getByRole('button', { name: 'Jobs' }).click();
  await page.locator('#jobs-drawer .jobs-drawer').getByRole('button', { name: 'View all' }).click();
  const topPager = page.getByTestId('jobs-pages-top');
  const bottomPager = page.getByTestId('jobs-pages-bottom');
  await expect(topPager).toBeVisible();
  await expect(bottomPager).toBeVisible();

  await page.keyboard.press('Shift+PageDown');
  await expect(topPager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(bottomPager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect.poll(() => offsets.includes(50)).toBe(true);
  expect(offsets.includes(100)).toBe(false);

  await page.keyboard.press('Shift+PageUp');
  await expect(topPager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');
});

async function openUpload(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

test('Shift+PageUp/PageDown navigates staged upload pages and respects boundaries', async ({ page }) => {
  await openUpload(page);
  await page.getByRole('group', { name: 'File upload drop zone' }).evaluate((zone) => {
    const transfer = new DataTransfer();
    for (let index = 0; index < 201; index += 1) {
      transfer.items.add(new File([new Uint8Array([index % 251])], `page_${String(index + 1).padStart(3, '0')}.jpg`, { type: 'image/jpeg' }));
    }
    zone.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }));
  });

  const pager = page.getByRole('navigation', { name: 'Staged upload pages' });
  const list = page.getByTestId('staged-upload-list');
  await expect(pager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');

  await page.keyboard.press('Shift+PageDown');
  await expect(pager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(list.getByText('page_101.jpg')).toBeVisible();

  await page.keyboard.press('Shift+PageDown');
  await expect(pager.getByRole('button', { name: 'Page 3', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(list.getByText('page_201.jpg')).toBeVisible();

  await page.keyboard.press('Shift+PageDown');
  await expect(pager.getByRole('button', { name: 'Page 3', exact: true })).toHaveAttribute('aria-current', 'page');

  await page.keyboard.press('Shift+PageUp');
  await expect(pager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page');
});
