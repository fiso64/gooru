import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function file(id: string) {
  return {
    id, content_id: 'hash-' + id, name: id + '.jpg',
    safe_display_path: 'library/' + id + '.jpg', size: 2048,
    modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg',
    media_kind: 'photo', metadata: { image_width: 800, image_height: 600 },
    tags: [], can_delete: true,
    media_urls: Object.fromEntries(['thumbnail', 'preview', 'content', 'download'].map((kind) =>
      [kind, '/api/v1/files/' + id + '/' + kind]))
  };
}

async function mockApp(page: Page, ids: string[], failRemoval = false, paged = false) {
  let available = ids.map(file);
  const removalModes: string[] = [];
  await page.route('**/api/v1/ui-config', (route) => route.fulfill({ json: paged ? { pagination_mode: 'paged', items_per_page: 1 } : {} }));
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({ json: session }));
  await page.route('**/api/v1/saved-searches', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/upload-targets', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/tags?**', (route) => route.fulfill({ json: { tags: [] } }));
  await page.route('**/api/v1/search/suggestions?**', (route) => route.fulfill({ json: { items: [] } }));
  await page.route('**/api/v1/files?**', (route) => {
    const params = new URL(route.request().url()).searchParams;
    const token = params.get('page_token');
    const offset = token ? Number(Buffer.from(token, 'base64url').toString().replace('offset:', '')) : 0;
    const limit = Number(params.get('limit') ?? available.length);
    return route.fulfill({ json: {
      files: available.slice(offset, offset + limit), total_count: available.length,
      library_count: available.length, facets: { kind: [] }
    } });
  });
  await page.route(/\/api\/v1\/files\/[^/?]+$/, (route) => {
    if (route.request().method() !== 'DELETE') return route.continue();
    const id = route.request().url().split('/').at(-1)!;
    removalModes.push(route.request().postDataJSON().mode);
    if (failRemoval) return route.fulfill({ status: 500, json: { error: { code: 'removal_failed', message: 'Removal refused' } } });
    available = available.filter((item) => item.id !== id);
    return route.fulfill({ json: {} });
  });
  await page.route(/\/api\/v1\/files\/[^/]+\/(thumbnail|preview|content)(\?.*)?$/, (route) =>
    route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"/>' }));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return removalModes;
}

async function removeViewedFile(page: Page, id: string, mode: 'untrack' | 'delete') {
  if (mode === 'delete') {
    await page.getByRole('button', { name: 'Delete ' + id + '.jpg from disk' }).click();
    await page.getByRole('dialog', { name: 'Delete file' }).getByRole('button', { name: 'Delete file' }).click();
  } else {
    await page.getByRole('button', { name: 'Untrack ' + id + '.jpg from library' }).click();
    await page.getByRole('dialog', { name: 'Remove from library' }).getByRole('button', { name: 'Remove', exact: true }).click();
  }
}

test('untracking the first visible item advances to the next remaining file', async ({ page }) => {
  const modes = await mockApp(page, ['one', 'two', 'three']);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await removeViewedFile(page, 'one', 'untrack');
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
  expect(modes).toEqual(['untrack']);
});

test('deleting the middle or last item selects the preceding item', async ({ page }) => {
  const modes = await mockApp(page, ['one', 'two', 'three']);
  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  await removeViewedFile(page, 'two', 'delete');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
  await page.keyboard.press('j');
  await expect(page.getByRole('dialog', { name: 'three.jpg' })).toBeVisible();
  await removeViewedFile(page, 'three', 'delete');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
  expect(modes).toEqual(['delete', 'delete']);
});

test('removing the sole file closes the viewer', async ({ page }) => {
  await mockApp(page, ['one']);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await removeViewedFile(page, 'one', 'untrack');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toHaveCount(0);
});

test('a failed removal leaves the viewer on the original file', async ({ page }) => {
  await mockApp(page, ['one', 'two'], true);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await removeViewedFile(page, 'one', 'delete');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
  await expect(page.getByText('Removal refused')).toBeVisible();
});

test('paged viewer resolves the preceding item across a page boundary', async ({ page }) => {
  await mockApp(page, ['one', 'two'], false, true);
  await page.getByRole('button', { name: 'Page 2' }).first().click();
  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  await removeViewedFile(page, 'two', 'untrack');
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Page 1' }).first()).toHaveAttribute('aria-current', 'page');
});
