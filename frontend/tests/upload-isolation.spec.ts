import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockAuth(page: Page) {
  let loggedIn = false;
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
}

async function mockShellApis(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) });
  });
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/upload-targets', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) });
  });
  await page.route('**/api/v1/tags?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/operations/job-*', async (route) => {
    const id = route.request().url().split('/').at(-1) ?? '';
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id, kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-09-02T00:00:00Z' })
    });
  });
}

async function signIn(page: Page) {
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('isolates browser upload failures per file and keeps siblings active', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);

  const uploadBodies: string[] = [];
  let successfulUploads = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    const body = (route.request().postDataBuffer() ?? Buffer.alloc(0)).toString();
    uploadBodies.push(body);
    if (body.includes('huge.mp4')) {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'invalid_multipart', message: 'multipart upload body is required' } })
      });
      return;
    }
    successfulUploads += 1;
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: `job-${successfulUploads}`, kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-09-02T00:00:00Z' })
    });
  });

  await page.goto('/');
  await signIn(page);
  await page.getByRole('button', { name: 'Upload' }).click();

  const chooser = page.locator('input[type="file"]');
  await chooser.setInputFiles([
    { name: 'small-a.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('small-a') },
    { name: 'huge.mp4', mimeType: 'video/mp4', buffer: Buffer.alloc(1024, 7) },
    { name: 'small-b.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('small-b') }
  ]);
  await page.getByRole('button', { name: /Upload 3 files/ }).click();

  await expect.poll(() => uploadBodies.length).toBe(3);
  expect(uploadBodies.some((body) => body.includes('small-a.jpg'))).toBe(true);
  expect(uploadBodies.some((body) => body.includes('huge.mp4'))).toBe(true);
  expect(uploadBodies.some((body) => body.includes('small-b.jpg'))).toBe(true);
  expect(uploadBodies.every((body) => ['small-a.jpg', 'huge.mp4', 'small-b.jpg'].filter((name) => body.includes(name)).length === 1)).toBe(true);

  const first = page.locator('.upload-row').filter({ hasText: 'small-a.jpg' });
  const failed = page.locator('.upload-row').filter({ hasText: 'huge.mp4' });
  const third = page.locator('.upload-row').filter({ hasText: 'small-b.jpg' });
  await expect(first.locator('.status')).toHaveText(/queued|importing/);
  await expect(failed).toContainText('multipart upload body is required');
  await expect(failed.locator('.status')).toHaveText('error');
  await expect(third.locator('.status')).toHaveText(/queued|importing/);
});
