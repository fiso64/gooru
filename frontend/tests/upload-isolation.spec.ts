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
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });
  await page.route('**/api/v1/jobs', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
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
  await page.route('**/api/v1/jobs/job-*', async (route) => {
    const id = route.request().url().split('/').at(-1) ?? '';
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id, type: 'upload_import', status: 'pending', submitted_at: '2026-09-02T00:00:00Z' })
    });
  });
}

async function signIn(page: Page) {
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

test('isolates browser upload failures per file and keeps siblings queued', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);

  const uploadBodies: Buffer[] = [];
  let requestIndex = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    uploadBodies.push(route.request().postDataBuffer() ?? Buffer.alloc(0));
    requestIndex += 1;
    if (requestIndex === 2) {
      await route.fulfill({
        status: 400,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'invalid_multipart', message: 'multipart upload body is required' } })
      });
      return;
    }
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ id: `job-${requestIndex}`, type: 'upload_import', status: 'pending', submitted_at: '2026-09-02T00:00:00Z' })
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
  expect(uploadBodies[0].toString()).toContain('small-a.jpg');
  expect(uploadBodies[0].toString()).not.toContain('huge.mp4');
  expect(uploadBodies[1].toString()).toContain('huge.mp4');
  expect(uploadBodies[2].toString()).toContain('small-b.jpg');

  const first = page.locator('.upload-row').filter({ hasText: 'small-a.jpg' });
  const failed = page.locator('.upload-row').filter({ hasText: 'huge.mp4' });
  const third = page.locator('.upload-row').filter({ hasText: 'small-b.jpg' });
  await expect(first).toContainText('queued');
  await expect(failed).toContainText('multipart upload body is required');
  await expect(failed).toContainText('error');
  await expect(third).toContainText('queued');
});
