import { expect, test, type Page, type Route } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type OperationStatus = 'pending' | 'completed';

function uploadResult(names: string[]) {
  return {
    files: names.map((name) => ({ name, size: 10, target_id: 'default', status: 'imported' })),
    affected_count: names.length
  };
}

function operationResponse(id: string, status: OperationStatus, names: string[] = [id.replace(/^job-/, '')]) {
  return {
    id,
    kind: 'upload_import',
    status,
    progress_total: names.length,
    progress_completed: status === 'completed' ? names.length : 0,
    progress_failed: 0,
    created_at: '2026-09-03T00:00:00Z',
    ...(status === 'completed' ? {
      finished_at: '2026-09-03T00:00:01Z',
      result: uploadResult(names)
    } : {})
  };
}

async function mockApp(
  page: Page,
  uploadOperationStatus: OperationStatus = 'pending',
  onBatchRequest?: (ids: string[]) => void,
  onLibraryRefresh?: (kind: 'jobs' | 'tags') => void,
  operationNames: string[] = []
) {
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
        body: JSON.stringify({ items: ids.map((id) => operationResponse(id, uploadOperationStatus, operationNames.length ? operationNames : undefined)) })
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
      body: JSON.stringify(operationResponse(id, uploadOperationStatus, operationNames.length ? operationNames : undefined))
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

async function fulfillUploadReservation(route: Route, id: string) {
  expect(route.request().headers()['x-gooru-upload-reserve']).toBe('true');
  await route.fulfill({
    status: 201,
    contentType: 'application/json',
    body: JSON.stringify(operationResponse(id, 'pending'))
  });
}

async function fulfillAggregateUpload(route: Route, id: string, names: string[]) {
  expect(route.request().headers()['prefer']).toBe('respond-async');
  expect(route.request().headers()['x-gooru-upload-operation-id']).toBe(id);
  await route.fulfill({
    status: 202,
    contentType: 'application/json',
    body: JSON.stringify(operationResponse(id, 'pending', names))
  });
}

test('browser submits one selected batch and releases it after durable admission', async ({ page }) => {
  const names = Array.from({ length: 7 }, (_, index) => `file-${index + 1}.jpg`);
  let statusBatchRequests = 0;
  await mockApp(page, 'pending', () => statusBatchRequests += 1, undefined, names);

  let active = 0;
  let maxActive = 0;
  let requestCount = 0;
  let reservationCount = 0;
  let waiting: Route | undefined;
  let uploadBody = '';

  await page.route('**/api/v1/uploads', async (route) => {
    if (route.request().headers()['x-gooru-upload-reserve'] === 'true') {
      reservationCount += 1;
      await fulfillUploadReservation(route, `reservation-${reservationCount}`);
      return;
    }
    requestCount += 1;
    active += 1;
    maxActive = Math.max(maxActive, active);
    waiting = route;
    uploadBody = route.request().postDataBuffer()?.toString('utf8') ?? '';
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles(
    names.map((name) => ({
      name,
      mimeType: 'image/jpeg',
      buffer: Buffer.from(name)
    }))
  );
  await page.getByRole('button', { name: /Upload 7 files/ }).click();

  await expect.poll(() => requestCount).toBe(1);
  expect(reservationCount).toBe(1);
  expect(active).toBe(1);
  expect(maxActive).toBe(1);
  for (const name of names) expect(uploadBody).toContain(`filename="${name}"`);

  expect(waiting).toBeDefined();
  active -= 1;
  await fulfillAggregateUpload(waiting!, 'reservation-1', names);
  await expect.poll(() => active).toBe(0);
  await expect(page.locator('.upload-row .status').filter({ hasText: 'queued' })).toHaveCount(names.length);
  await expect(page.getByTestId('upload-queue-batch').locator('.upload-batch-progress')).toBeVisible();
  await expect.poll(() => statusBatchRequests).toBeGreaterThan(0);
  expect(requestCount).toBe(1);
});

test('large staged WebUI upload completes through one durable operation lookup', async ({ page }) => {
  const total = 130;
  const names = Array.from({ length: total }, (_, index) => `batch-${index + 1}.jpg`);
  let statusBatchRequests = 0;
  let largestStatusBatch = 0;
  await mockApp(page, 'completed', (ids) => {
    statusBatchRequests += 1;
    largestStatusBatch = Math.max(largestStatusBatch, ids.length);
  }, undefined, names);

  let requestCount = 0;
  let reservationCount = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    if (route.request().headers()['x-gooru-upload-reserve'] === 'true') {
      reservationCount += 1;
      await fulfillUploadReservation(route, `reservation-${reservationCount}`);
      return;
    }
    requestCount += 1;
    await fulfillAggregateUpload(route, 'reservation-1', names);
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles(
    names.map((name) => ({
      name,
      mimeType: 'image/jpeg',
      buffer: Buffer.from(name)
    }))
  );
  await page.getByRole('button', { name: /Upload 130 files/ }).click();

  await expect.poll(() => requestCount).toBe(1);
  expect(reservationCount).toBe(1);
  await expect(page.locator('[data-testid="upload-queue-list"] .status').filter({ hasText: 'imported' })).toHaveCount(100, { timeout: 5000 });
  await page.getByRole('button', { name: 'Next' }).click();
  await expect(page.locator('[data-testid="upload-queue-list"] .status').filter({ hasText: 'imported' })).toHaveCount(30, { timeout: 5000 });

  expect(statusBatchRequests).toBeGreaterThan(0);
  expect(largestStatusBatch).toBe(1);
  expect(requestCount).toBe(1);
});


test('submits a newer batch while an older browser request is still in flight and groups it first', async ({ page }) => {
  await mockApp(page, 'pending');

  let reservationCount = 0;
  const waiting: Route[] = [];
  await page.route('**/api/v1/uploads', async (route) => {
    if (route.request().headers()['x-gooru-upload-reserve'] === 'true') {
      reservationCount += 1;
      await fulfillUploadReservation(route, `reservation-${reservationCount}`);
      return;
    }
    waiting.push(route);
  });

  await signIn(page);
  const input = page.locator('input[type="file"]');
  await input.setInputFiles({ name: 'first.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('first') });
  await page.getByRole('button', { name: /Upload 1 file/ }).click();
  await expect.poll(() => waiting.length).toBe(1);

  await input.setInputFiles({ name: 'second.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('second') });
  const secondSubmit = page.getByRole('button', { name: /Upload 1 file/ });
  await expect(secondSubmit).toBeEnabled();
  await secondSubmit.click();
  await expect.poll(() => waiting.length).toBe(2);
  expect(reservationCount).toBe(2);

  await fulfillAggregateUpload(waiting[1]!, 'reservation-2', ['second.jpg']);
  await fulfillAggregateUpload(waiting[0]!, 'reservation-1', ['first.jpg']);

  const batches = page.getByTestId('upload-queue-batch');
  await expect(batches).toHaveCount(2);
  await expect(batches.nth(0)).toHaveAttribute('data-batch-id', '2');
  await expect(batches.nth(0)).toContainText('second.jpg');
  await expect(batches.nth(1)).toHaveAttribute('data-batch-id', '1');
  await expect(batches.nth(1)).toContainText('first.jpg');
  await expect(batches.nth(0).locator('.upload-batch-progress')).toBeHidden();
  await expect(batches.nth(1).locator('.upload-batch-progress')).toBeHidden();
  await expect(page.getByRole('button', { name: 'Pause all' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Clear done' })).toHaveCount(1);
  await expect(page.getByRole('button', { name: /^Cancel$/ })).toHaveCount(1);
});
