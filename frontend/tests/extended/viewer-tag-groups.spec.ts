import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(tags: string[]) {
  return {
    id: 'one',
    content_id: 'hash-one',
    name: 'one.jpg',
    safe_display_path: 'library/one.jpg',
    size: 2048,
    added_at: '2026-05-19T00:00:00Z',
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 },
    tags,
    media_urls: {
      thumbnail: '/api/v1/files/one/thumbnail',
      preview: '/api/v1/files/one/preview',
      content: '/api/v1/files/one/content',
      download: '/api/v1/files/one/download'
    }
  };
}

async function mockApp(page: Page, initialTags: string[] = []) {
  let loggedIn = false;
  let serverTags = [...initialTags];

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: loggedIn ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
    });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: serverTags.map((name) => ({ name, count: 1 })), library_count: 1, facets: { kind: [] } }) }));
  await page.route('**/api/v1/jobs?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], total_count: 0, active_count: 0 }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [fileItem(serverTags)], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/tags', async (route) => {
    const request = route.request();
    const body = request.postDataJSON() as { tags?: string[] };
    if (request.method() === 'POST') {
      serverTags = Array.from(new Set([...serverTags, ...(body.tags ?? [])]));
    }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ matched_files: 1, changed_files: 1 }) });
  });
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>' }));
  await page.route('**/api/v1/files/one/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>' }));
  await page.route('**/api/v1/files/one/content', async (route) => route.fulfill({ status: 404, contentType: 'text/plain', body: 'fixture unavailable' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();
}

test('viewer keeps OTHER last while tags are added live', async ({ page }) => {
  await mockApp(page);
  const dialog = page.getByRole('dialog', { name: 'one.jpg' });
  const input = page.getByLabel('Tags for one.jpg');

  for (const tag of ['key1:value1', 'tag', 'key2:value2']) {
    await input.fill(tag);
    await input.press('Enter');
    await expect(input).toHaveValue('');
    await expect(dialog.getByRole('button', { name: `Search for ${tag}` })).toBeVisible();
  }

  const groups = dialog.locator('.lightbox-tag-group');
  await expect(groups).toHaveCount(3);
  await expect(groups.nth(0).locator('.lightbox-tag-group-head span').first()).toHaveText('key1');
  await expect(groups.nth(1).locator('.lightbox-tag-group-head span').first()).toHaveText('key2');
  await expect(groups.nth(2).locator('.lightbox-tag-group-head span').first()).toHaveText('OTHER');
  await expect(groups.nth(2).getByRole('button', { name: 'Search for tag' })).toBeVisible();
});

test('viewer omits OTHER header when there are no namespace groups', async ({ page }) => {
  await mockApp(page, ['plain']);
  const dialog = page.getByRole('dialog', { name: 'one.jpg' });
  await expect(dialog.getByText('OTHER', { exact: true })).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: 'Search for plain' })).toBeVisible();
});
