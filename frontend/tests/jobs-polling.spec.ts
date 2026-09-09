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
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('does not keep polling operations on idle screens', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  let operationRequests = 0;
  await page.route('**/api/v1/operations?**', async (route) => {
    operationRequests += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });

  await signIn(page);
  await page.getByRole('button', { name: 'Tags' }).click();
  await expect(page.getByRole('heading', { name: 'Tags' })).toBeVisible();
  await expect.poll(() => operationRequests).toBeGreaterThan(0);
  await page.waitForTimeout(200);
  const settledRequests = operationRequests;
  await page.waitForTimeout(2300);
  expect(operationRequests).toBe(settledRequests);
});

test('keeps polling while an operation is active', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  let operationRequests = 0;
  await page.route('**/api/v1/operations?**', async (route) => {
    operationRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [{ id: 'op-1', kind: 'delete_files', status: 'running', progress_total: 2, progress_completed: 1, progress_failed: 0, created_at: '2026-09-08T08:00:00Z' }] })
    });
  });

  await signIn(page);
  await page.waitForTimeout(200);
  const initialRequests = operationRequests;
  await expect.poll(() => operationRequests, { timeout: 3500 }).toBeGreaterThan(initialRequests);
});
