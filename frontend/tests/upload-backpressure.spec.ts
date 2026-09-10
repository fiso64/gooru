import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function uploadResult(name: string) {
  return {
    files: [{ name, size: 10, target_id: 'default', status: 'imported' }],
    affected_count: 1
  };
}

function operationResponse(id: string, completed: boolean) {
  const name = id.replace(/^job-/, '');
  return {
    id,
    kind: 'upload_import',
    status: completed ? 'completed' : 'pending',
    progress_total: 1,
    progress_completed: completed ? 1 : 0,
    progress_failed: 0,
    created_at: '2026-09-07T00:00:00Z',
    ...(completed ? {
      finished_at: '2026-09-07T00:00:01Z',
      result: uploadResult(name)
    } : {})
  };
}

async function mockApp(page: Page, jobsCompleted: () => boolean) {
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
  await page.route('**/api/v1/operations?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify(ids.length
        ? { items: ids.map((id) => operationResponse(id, jobsCompleted())) }
        : { items: [], active_count: 0 })
    });
  });
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

function testFiles(total: number) {
  return Array.from({ length: total }, (_, index) => ({
    name: `batch-${index + 1}.jpg`,
    mimeType: 'image/jpeg',
    buffer: Buffer.from(`batch-${index + 1}`)
  }));
}

test('large upload pauses admission while accepted import jobs remain pending', async ({ page }) => {
  let completed = false;
  await mockApp(page, () => completed);

  let requestCount = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    requestCount += 1;
    const name = `batch-${requestCount}.jpg`;
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify(operationResponse(`job-${name}`, false))
    });
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles(testFiles(70));
  await page.getByRole('button', { name: /Upload 70 files/ }).click();

  await expect.poll(() => requestCount).toBeGreaterThanOrEqual(64);
  await page.waitForTimeout(600);
  expect(requestCount).toBeLessThan(70);
  expect(requestCount).toBeLessThanOrEqual(67);

  completed = true;
  await expect.poll(() => requestCount, { timeout: 5000 }).toBe(70);
  await expect(page.locator('.upload-row .status').filter({ hasText: 'imported' })).toHaveCount(70, { timeout: 5000 });
});

test('durable admission saturation is transient backpressure instead of a failed upload', async ({ page }) => {
  await mockApp(page, () => true);

  let requestCount = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    requestCount += 1;
    if (requestCount === 1) {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'job_queue_full', message: 'job queue is full' } })
      });
      return;
    }
    const name = `batch-${requestCount - 1}.jpg`;
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify(operationResponse(`job-${name}`, false))
    });
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles(testFiles(1));
  await page.getByRole('button', { name: /Upload 1 file/ }).click();

  await expect.poll(() => requestCount, { timeout: 5000 }).toBe(2);
  await expect(page.locator('.upload-row .status')).toContainText('imported', { timeout: 5000 });
  await expect(page.locator('.upload-row .error')).toHaveCount(0);
});
