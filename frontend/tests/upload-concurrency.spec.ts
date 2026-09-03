import { expect, test, type Page, type Route } from '@playwright/test';

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
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/jobs/job-*', async (route) => {
    const id = route.request().url().split('/').at(-1) ?? '';
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ id, type: 'upload_import', status: 'pending', submitted_at: '2026-09-03T00:00:00Z' })
    });
  });
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

async function fulfillUpload(route: Route, id: number) {
  await route.fulfill({
    status: 202,
    contentType: 'application/json',
    body: JSON.stringify({ id: `job-${id}`, type: 'upload_import', status: 'pending', submitted_at: '2026-09-03T00:00:00Z' })
  });
}

test('browser keeps at most four upload requests in flight and drains queued files', async ({ page }) => {
  await mockApp(page);

  let active = 0;
  let maxActive = 0;
  let requestCount = 0;
  const waiting: Array<{ route: Route; id: number }> = [];

  await page.route('**/api/v1/uploads', async (route) => {
    requestCount += 1;
    active += 1;
    maxActive = Math.max(maxActive, active);
    waiting.push({ route, id: requestCount });
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles(
    Array.from({ length: 7 }, (_, index) => ({
      name: `file-${index + 1}.jpg`,
      mimeType: 'image/jpeg',
      buffer: Buffer.from(`file-${index + 1}`)
    }))
  );
  await page.getByRole('button', { name: /Upload 7 files/ }).click();

  await expect.poll(() => requestCount).toBe(4);
  await page.waitForTimeout(100);
  expect(requestCount).toBe(4);
  expect(active).toBe(4);
  expect(maxActive).toBe(4);

  const first = waiting.shift();
  expect(first).toBeDefined();
  active -= 1;
  await fulfillUpload(first!.route, first!.id);
  await expect.poll(() => requestCount).toBe(5);
  expect(active).toBe(4);

  const second = waiting.shift();
  expect(second).toBeDefined();
  active -= 1;
  await fulfillUpload(second!.route, second!.id);
  await expect.poll(() => requestCount).toBe(6);
  expect(active).toBe(4);

  const third = waiting.shift();
  expect(third).toBeDefined();
  active -= 1;
  await fulfillUpload(third!.route, third!.id);
  await expect.poll(() => requestCount).toBe(7);
  expect(active).toBe(4);

  while (waiting.length) {
    const next = waiting.shift()!;
    active -= 1;
    await fulfillUpload(next.route, next.id);
  }

  await expect.poll(() => active).toBe(0);
  expect(maxActive).toBe(4);
  await expect(page.locator('.upload-row .status').filter({ hasText: /queued|importing/ })).toHaveCount(7);
});
