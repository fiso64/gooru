import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

test('authenticated startup does not refetch global queries after mount', async ({ page }) => {
  const requests = { operations: 0, tags: 0, savedSearches: 0, uploadTargets: 0 };
  let facetFileRequests = 0;

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => {
    if (new URL(route.request().url()).searchParams.get('include_facets') === 'true') facetFileRequests += 1;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
    });
  });
  await page.route('**/api/v1/operations?**', async (route) => {
    requests.operations += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/tags?**', async (route) => {
    requests.tags += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [], library_count: 0 }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => {
    requests.savedSearches += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });
  await page.route('**/api/v1/upload-targets', async (route) => {
    requests.uploadTargets += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) });
  });

  await page.goto('/');
  await expect.poll(() => Math.min(...Object.values(requests))).toBe(1);
  await page.waitForTimeout(300);
  expect(requests).toEqual({ operations: 1, tags: 1, savedSearches: 1, uploadTargets: 1 });
  expect(facetFileRequests).toBe(1);
});
