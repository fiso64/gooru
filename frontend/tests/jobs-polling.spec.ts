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
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/tags?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('does not keep polling jobs on idle screens', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  let jobsRequests = 0;
  await page.route('**/api/v1/jobs', async (route) => {
    jobsRequests += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });

  await signIn(page);
  await page.getByRole('button', { name: 'Tags' }).click();
  await expect(page.getByRole('heading', { name: 'Tags' })).toBeVisible();
  await expect.poll(() => jobsRequests).toBeGreaterThan(0);
  await page.waitForTimeout(200);
  const settledRequests = jobsRequests;
  await page.waitForTimeout(2300);
  expect(jobsRequests).toBe(settledRequests);
});

test('keeps polling while a job is active', async ({ page }) => {
  await mockAuth(page);
  await mockShellApis(page);
  let jobsRequests = 0;
  await page.route('**/api/v1/jobs', async (route) => {
    jobsRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [{ id: 'job-1', type: 'upload_import', status: 'running', progress: 0.5 }] })
    });
  });

  await signIn(page);
  await page.waitForTimeout(200);
  const initialRequests = jobsRequests;
  await expect.poll(() => jobsRequests, { timeout: 3500 }).toBeGreaterThan(initialRequests);
});
