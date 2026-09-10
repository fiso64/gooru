import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type OperationStatus = 'pending' | 'completed';

function uploadResult(name: string) {
  return {
    files: [{ name, size: 10, target_id: 'default', status: 'imported' }],
    affected_count: 1
  };
}

function operationResponse(id: string, status: OperationStatus) {
  const name = id.replace(/^job-/, '');
  return {
    id,
    kind: 'upload_import',
    status,
    progress_total: 1,
    progress_completed: status === 'completed' ? 1 : 0,
    progress_failed: 0,
    created_at: '2026-09-03T00:00:00Z',
    ...(status === 'completed' ? {
      finished_at: '2026-09-03T00:00:01Z',
      result: uploadResult(name)
    } : {})
  };
}

async function mockApp(page: Page, uploadOperationStatus: OperationStatus = 'pending', onBatchRequest?: (ids: string[]) => void, onLibraryRefresh?: (kind: 'jobs' | 'tags') => void) {
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
  await page.route('**/api/v1/operations', async (route) => {
    onLibraryRefresh?.('jobs');
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/operations?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    if (ids.length) {
      onBatchRequest?.(ids);
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ items: ids.map((id) => operationResponse(id, uploadOperationStatus)) })
      });
      return;
    }
    onLibraryRefresh?.('jobs');
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => { onLibraryRefresh?.('tags'); await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }); });
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/operations/job-*', async (route) => {
    const id = decodeURIComponent(route.request().url().split('/').at(-1) ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(operationResponse(id, uploadOperationStatus))
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

async function fulfillStagedUpload(route: Route, name: string) {
  expect(route.request().headers()['prefer']).toBe('respond-async');
  await route.fulfill({
    status: 202,
    contentType: 'application/json',
    body: JSON.stringify(operationResponse(`job-${name}`, 'pending'))
  });
}

test('browser releases transfer slots after staging while durable imports remain pending', async ({ page }) => {
  let statusBatchRequests = 0;
  await mockApp(page, 'pending', () => statusBatchRequests += 1);

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
  await fulfillStagedUpload(first!.route, `file-${first!.id}.jpg`);
  await expect.poll(() => requestCount).toBe(5);
  await expect(page.locator('.upload-row').nth(first!.id - 1).locator('.status')).toContainText('queued');
  expect(active).toBe(4);

  const second = waiting.shift();
  expect(second).toBeDefined();
  active -= 1;
  await fulfillStagedUpload(second!.route, `file-${second!.id}.jpg`);
  await expect.poll(() => requestCount).toBe(6);
  await expect(page.locator('.upload-row').nth(second!.id - 1).locator('.status')).toContainText('queued');
  expect(active).toBe(4);

  const third = waiting.shift();
  expect(third).toBeDefined();
  active -= 1;
  await fulfillStagedUpload(third!.route, `file-${third!.id}.jpg`);
  await expect.poll(() => requestCount).toBe(7);
  await expect(page.locator('.upload-row').nth(third!.id - 1).locator('.status')).toContainText('queued');
  expect(active).toBe(4);

  while (waiting.length) {
    const next = waiting.shift()!;
    active -= 1;
    await fulfillStagedUpload(next.route, `file-${next.id}.jpg`);
  }

  await expect.poll(() => active).toBe(0);
  expect(maxActive).toBe(4);
  await expect(page.locator('.upload-row .status').filter({ hasText: 'queued' })).toHaveCount(7);
  await expect.poll(() => statusBatchRequests).toBeGreaterThan(0);
});

test('large staged WebUI uploads complete through bounded batched durable-operation polling', async ({ page }) => {
  let statusBatchRequests = 0;
  await mockApp(page, 'completed', () => statusBatchRequests += 1);

  let requestCount = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    requestCount += 1;
    await fulfillStagedUpload(route, `batch-${requestCount}.jpg`);
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

  expect(statusBatchRequests).toBeGreaterThan(0);
  expect(statusBatchRequests).toBeLessThanOrEqual(Math.ceil(total / 64) + 1);
});