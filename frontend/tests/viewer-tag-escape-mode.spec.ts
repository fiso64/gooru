import { expect, test } from '@playwright/test';

test('empty viewer tag input blurs on Escape and +/- changes modes only while empty', async ({ page }) => {
  let loggedIn = false;
  const session = { user: { id: 'usr_test', username: 'mac', role: 'admin' }, capabilities: { upload: true, tag: true, delete: true, admin: true }, csrf_token: 'csrf-one' };
  const file = { id: 'one', content_id: 'hash-one', name: 'one.jpg', safe_display_path: 'library/one.jpg', size: 2048,
    modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo', can_delete: true,
    metadata: { image_width: 800, image_height: 600 }, tags: [],
    media_urls: { thumbnail: '/api/v1/files/one/thumbnail', preview: '/api/v1/files/one/preview', content: '/api/v1/files/one/content', download: '/api/v1/files/one/download' } };
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: loggedIn ? 200 : 401, contentType: 'application/json', body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } }) }));
  await page.route('**/api/v1/auth/login', async (route) => { loggedIn = true; await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }); });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: '{"tags":[]}' }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: '{"items":[]}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } }) }));
  await page.route('**/api/v1/files/one/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  await page.route('**/api/v1/files/one/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64" />' }));
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();

  const add = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await add.focus();
  await add.press('Escape');
  await expect(add).not.toBeFocused();
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();

  await add.focus();
  await add.press('-');
  const remove = page.getByRole('textbox', { name: 'Remove tags from one.jpg' });
  await expect(remove).toBeFocused();
  await expect(remove).toHaveValue('');
  await remove.press('+');
  await expect(add).toBeFocused();
  await expect(add).toHaveValue('');

  await add.fill('rating:safe');
  await add.press('-');
  await expect(add).toHaveValue('rating:safe-');
});
