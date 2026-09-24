import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const file = {
  id: 'one',
  content_id: 'hash-one',
  name: 'one.jpg',
  safe_display_path: 'library/one.jpg',
  size: 2048,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'image/jpeg',
  media_kind: 'photo',
  metadata: { image_width: 800, image_height: 600 },
  tags: [],
  media_urls: {
    thumbnail: '/api/v1/files/one/thumbnail',
    preview: '/api/v1/files/one/preview',
    content: '/api/v1/files/one/content',
    download: '/api/v1/files/one/download'
  }
};

async function mockApp(page: Page) {
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
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [{ name: 'alpha', value: 'alpha', namespace: '', count: 1 }] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('plain Enter accepts a no-input confirmation dialog', async ({ page }) => {
  await mockApp(page);
  const removals: unknown[] = [];
  await page.route('**/api/v1/files', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    removals.push(route.request().postDataJSON());
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ mode: 'delete', selector: removals.at(-1), removed_locations: 1 }) });
  });

  await page.keyboard.press('a');
  await page.keyboard.press('Shift+Delete');
  const dialog = page.getByRole('dialog', { name: 'Delete selected files' });
  await expect(dialog).toBeVisible();
  await expect(dialog).toBeFocused();

  await page.keyboard.press('Enter');
  await expect.poll(() => removals.length).toBe(1);
  expect(removals[0]).toEqual({ mode: 'delete', query: '*' });
  await expect(dialog).toHaveCount(0);
});

test('plain Enter stays inside tag input while Ctrl+Enter accepts the modal', async ({ page }) => {
  await mockApp(page);
  const tagRequests: unknown[] = [];
  await page.route('**/api/v1/files/tags', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    tagRequests.push(route.request().postDataJSON());
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ updated_files: 1 }) });
  });

  await page.keyboard.press('a');
  await page.keyboard.press('t');
  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  const input = page.getByRole('textbox', { name: 'Tags', exact: true });
  await expect(dialog).toBeVisible();
  await expect(input).toBeFocused();

  await input.fill('alpha');
  await input.press('Enter');
  await expect(dialog).toBeVisible();
  await expect(page.getByRole('button', { name: 'Remove alpha' })).toBeVisible();
  expect(tagRequests).toHaveLength(0);

  await input.press('Control+Enter');
  await expect.poll(() => tagRequests.length).toBe(1);
  await expect(dialog).toHaveCount(0);
});
