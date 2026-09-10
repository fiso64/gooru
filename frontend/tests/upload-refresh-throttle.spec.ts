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

async function mockApp(page: Page) {
  let loggedIn = false;
  let tagsRequests = 0;
  let uploadRequests = 0;
  let jobPolls = 0;

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
  await page.route('**/api/v1/files?**', async (route) => {
    const params = new URL(route.request().url()).searchParams;
    const includeFacets = params.get('include_facets') === 'true';
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [],
        total_count: 12,
        ...(includeFacets ? { library_count: 12, facets: { kind: [{ value: 'image', count: 12 }] } } : {})
      })
    });
  });
  await page.route('**/api/v1/operations?**', async (route) => {
    const ids = new URL(route.request().url()).searchParams.getAll('id');
    if (!ids.length) {
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], active_count: 0 }) });
      return;
    }
    jobPolls += 1;
    const completeCount = Math.min(ids.length, jobPolls * 2);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: ids.map((id, index) => operationResponse(id, index < completeCount))
      })
    });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => {
    tagsRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ tags: [], library_count: 12, facets: { kind: [{ value: 'image', count: 12 }] } })
    });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/uploads', async (route) => {
    uploadRequests += 1;
    const name = `batch-${uploadRequests}.jpg`;
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify(operationResponse(`job-${name}`, false))
    });
  });

  return {
    tagsRequests: () => tagsRequests,
    uploadRequests: () => uploadRequests,
    jobPolls: () => jobPolls
  };
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

test('async import completions coalesce sidebar metadata refreshes', async ({ page }) => {
  const server = await mockApp(page);
  await signIn(page);
  await expect.poll(() => server.tagsRequests()).toBeGreaterThan(0);
  const baselineTags = server.tagsRequests();

  await page.locator('input[type="file"]').setInputFiles(
    Array.from({ length: 12 }, (_, index) => ({
      name: `batch-${index + 1}.jpg`,
      mimeType: 'image/jpeg',
      buffer: Buffer.from(`batch-${index + 1}`)
    }))
  );
  await page.getByRole('button', { name: /Upload 12 files/ }).click();

  await expect.poll(() => server.uploadRequests()).toBe(12);
  await expect.poll(() => server.jobPolls(), { timeout: 5_000 }).toBeGreaterThanOrEqual(3);
  await expect(page.locator('.upload-row .status').filter({ hasText: 'imported' })).toHaveCount(12, { timeout: 5_000 });

  await expect.poll(() => server.tagsRequests(), { timeout: 2_000 }).toBeGreaterThan(baselineTags);
  await page.waitForTimeout(300);
  expect(server.tagsRequests() - baselineTags).toBe(1);
});
