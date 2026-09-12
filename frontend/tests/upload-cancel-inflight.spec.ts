import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
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
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/operations?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/operations', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

test('cancel aborts an upload request that is still in flight before durable admission', async ({ page }) => {
  await mockApp(page);

  let uploadRequests = 0;
  await page.route('**/api/v1/uploads', async () => {
    uploadRequests += 1;
    // Keep the request in flight, matching the owner repro where the first large file
    // is still transferring and the server has not returned a durable operation yet.
    await new Promise<void>(() => {});
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles([
    { name: 'first.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('first') },
    { name: 'second.bin', mimeType: 'application/octet-stream', buffer: Buffer.from('second') }
  ]);
  await page.getByRole('button', { name: /Upload 2 files/ }).click();

  await expect.poll(() => uploadRequests).toBe(1);
  await expect(page.getByRole('button', { name: /^Cancel$/ })).toBeVisible();
  await page.getByRole('button', { name: /^Cancel$/ }).click();

  await expect(page.locator('.upload-row .status').filter({ hasText: 'canceled' })).toHaveCount(2, { timeout: 2_000 });
  await expect(page.getByRole('button', { name: /^Cancel$/ })).toHaveCount(0);
});
