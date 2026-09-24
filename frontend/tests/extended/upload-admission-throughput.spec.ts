import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

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
      result: { files: [{ name, size: 1, target_id: 'default', status: 'imported' }], affected_count: 1 }
    } : {})
  };
}

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

  const seenJobPolls = new Set<string>();
  await page.route('**/api/v1/operations?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    const items = ids.map((id) => {
      const completed = seenJobPolls.has(id);
      seenJobPolls.add(id);
      return operationResponse(id, completed);
    });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items }) });
  });

  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
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
    name: `tiny-${String(index + 1).padStart(4, '0')}.jpg`,
    mimeType: 'image/jpeg',
    buffer: Buffer.from([index % 251])
  }));
}

test('1K fast uploads replenish the bounded admission window without coarse stalls', async ({ page }) => {
  test.setTimeout(20_000);
  await mockApp(page);

  const requestTimes: number[] = [];
  await page.route('**/api/v1/uploads', async (route) => {
    requestTimes.push(Date.now());
    const index = requestTimes.length;
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify(operationResponse(`job-tiny-${String(index).padStart(4, '0')}.jpg`, false))
    });
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles(testFiles(1000));
  await page.getByRole('button', { name: /Upload 1000 files/ }).click();

  await expect.poll(() => requestTimes.length, { timeout: 12_000 }).toBe(1000);

  const gaps = requestTimes.slice(1).map((time, index) => time - requestTimes[index]);
  // Each status window deliberately needs a second poll before completion. The
  // old 250 ms admission sleeps (and 700 ms status cadence) therefore create a
  // visible refill gap; completion-driven wakeups should keep every refill well
  // below that coarse retry interval.
  expect(Math.max(...gaps)).toBeLessThan(200);
});
