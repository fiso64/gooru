import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function openUploadPanel(page: Page) {
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
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
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

test('WebUI hides conflict policy choices and uploads with rename', async ({ page }) => {
  let requestBody = '';
  await page.route('**/api/v1/uploads', async (route) => {
    requestBody = route.request().postDataBuffer()?.toString('utf8') ?? '';
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'job-1', kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-09-05T00:00:00Z' })
    });
  });
  await page.route('**/api/v1/operations/job-*', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ id: 'job-1', kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-09-05T00:00:00Z' })
  }));

  await openUploadPanel(page);
  await expect(page.getByText('On conflict', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('combobox').filter({ has: page.locator('option[value="rename"]') })).toHaveCount(0);

  await page.locator('input[type="file"]').setInputFiles({
    name: 'same-name.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('upload')
  });
  await page.getByRole('button', { name: /Upload 1 file/ }).click();

  await expect.poll(() => requestBody).not.toBe('');
  expect(requestBody).toContain('name="conflict_policy"');
  expect(requestBody).toMatch(/name="conflict_policy"\r?\n\r?\nrename/);
  expect(requestBody).not.toMatch(/name="conflict_policy"\r?\n\r?\n(skip|replace)/);
});
