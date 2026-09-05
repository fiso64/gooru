import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type JobStatus = 'pending' | 'completed';

function jobResponse(id: string, status: JobStatus) {
  return {
    id,
    type: 'upload_import',
    status,
    progress: status === 'completed' ? 1 : 0,
    submitted_at: '2026-09-03T00:00:00Z',
    ...(status === 'completed' ? { finished_at: '2026-09-03T00:00:01Z' } : {})
  };
}

async function mockApp(page: Page, uploadJobStatus: JobStatus = 'pending', onBatchRequest?: (ids: string[]) => void) {
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
  await page.route('**/api/v1/jobs?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    onBatchRequest?.(ids);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: ids.map((id) => jobResponse(id, uploadJobStatus)) })
    });
  });
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
      body: JSON.stringify(jobResponse(id, uploadJobStatus))
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
    body: JSON.stringify(jobResponse(`job-${id}`, 'pending'))
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

test('completed async imports reconcile in bounded batches instead of one job per poll interval', async ({ page }) => {
  const batchSizes: number[] = [];
  await mockApp(page, 'completed', (ids) => batchSizes.push(ids.length));

  let requestCount = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    requestCount += 1;
    await fulfillUpload(route, requestCount);
  });

  await signIn(page);
  const total = 130;
  await page.locator('input[type="file"]').setInputFiles(
    Array.from({ length: total }, (_, index) => ({
      name: `batch-${index + 1}.jpg`,
      mimeType: 'image/jpeg',
      buffer: Buffer.from(`batch-${index + 1}`)
    }))
  );
  await page.getByRole('button', { name: /Upload 130 files/ }).click();

  await expect.poll(() => requestCount).toBe(total);
  await expect(page.locator('[data-testid="upload-queue-list"] .status').filter({ hasText: 'imported' })).toHaveCount(100, { timeout: 5000 });
  await page.getByRole('button', { name: 'Next' }).click();
  await expect(page.locator('[data-testid="upload-queue-list"] .status').filter({ hasText: 'imported' })).toHaveCount(30, { timeout: 5000 });

  expect(batchSizes.length).toBeGreaterThan(0);
  expect(Math.max(...batchSizes)).toBeLessThanOrEqual(64);
  expect(batchSizes.length).toBeLessThan(20);
});
