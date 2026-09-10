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

async function signIn(page: Page) {
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
}

async function mockBackgroundApis(page: Page) {
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) =>
    route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) })
  );
}

test('persists saved-search drag order across reload', async ({ page }) => {
  await mockAuth(page);
  await mockBackgroundApis(page);

  let saved = [
    { id: 's1', name: 'First', query: 'one', sort: 'name', order: 'asc', created_at: '2026-09-01T00:00:00Z', updated_at: '2026-09-01T00:00:00Z' },
    { id: 's2', name: 'Second', query: 'two', sort: 'name', order: 'asc', created_at: '2026-09-01T00:00:01Z', updated_at: '2026-09-01T00:00:01Z' },
    { id: 's3', name: 'Third', query: 'three', sort: 'name', order: 'asc', created_at: '2026-09-01T00:00:02Z', updated_at: '2026-09-01T00:00:02Z' }
  ];
  const reorderRequests: Array<{ ids: string[]; csrf: string }> = [];

  await page.route('**/api/v1/saved-searches/reorder', async (route) => {
    const body = route.request().postDataJSON() as { ids: string[] };
    reorderRequests.push({ ids: body.ids, csrf: route.request().headers()['x-gooru-csrf'] ?? '' });
    saved = body.ids.map((id) => saved.find((item) => item.id === id)!);
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: saved }) });
  });

  await page.goto('/');
  await signIn(page);

  const rows = page.locator('.sidebar-saved-row');
  await expect(rows.locator('.truncate')).toHaveText(['First', 'Second', 'Third']);

  await expect(page.getByRole('button', { name: 'Drag First' })).toHaveCount(0);
  const firstEntry = rows.filter({ hasText: 'First' }).locator('.sidebar-item');
  const thirdRow = rows.filter({ hasText: 'Third' });
  await expect(firstEntry).toHaveAttribute('draggable', 'true');

  const dataTransfer = await page.evaluateHandle(() => new DataTransfer());
  const thirdBox = await thirdRow.boundingBox();
  expect(thirdBox).not.toBeNull();
  await firstEntry.dispatchEvent('dragstart', { dataTransfer });
  await thirdRow.dispatchEvent('dragover', {
    dataTransfer,
    clientY: thirdBox!.y + thirdBox!.height - 1
  });

  await expect(rows.locator('.truncate')).toHaveText(['Second', 'Third', 'First']);
  expect(reorderRequests).toHaveLength(0);

  await thirdRow.dispatchEvent('drop', {
    dataTransfer,
    clientY: thirdBox!.y + thirdBox!.height - 1
  });
  await firstEntry.dispatchEvent('dragend', { dataTransfer });

  await expect.poll(() => reorderRequests).toEqual([{ ids: ['s2', 's3', 's1'], csrf: 'csrf-one' }]);
  await expect(rows.locator('.truncate')).toHaveText(['Second', 'Third', 'First']);

  await page.reload();
  await expect(page.locator('.sidebar-saved-row .truncate')).toHaveText(['Second', 'Third', 'First']);
});
