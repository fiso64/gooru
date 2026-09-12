import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page) {
  let loggedIn = false;
  let dedicatedComicCountRequests = 0;

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [], library_count: 3, facets: { kind: [{ value: 'comic', count: 2 }] } })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] })
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'saved-zero', name: 'No comics', query: 'tag:no-comics', sort: 'modified', order: 'desc' }] })
  }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const query = url.searchParams.get('query') ?? '';
    if (query === 'ext:cbz' && url.searchParams.get('limit') === '1') dedicatedComicCountRequests += 1;
    const includeFacets = url.searchParams.get('include_facets') === 'true';
    const total = query === 'ext:cbz' ? 2 : query.includes('tag:no-comics') ? 0 : 3;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files: [],
        total_count: total,
        library_count: 3,
        facets: includeFacets ? { kind: total > 0 && !query.includes('tag:no-comics') ? [{ value: 'comic', count: 2 }] : [] } : undefined
      })
    });
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return () => dedicatedComicCountRequests;
}

test('comics entry stays visible while its count follows the active saved search', async ({ page }) => {
  const dedicatedComicCountRequests = await mockApp(page);

  const comics = page.locator('.sidebar-item[data-sidebar-shortcut]').filter({ hasText: 'Comics' });
  await expect(comics).toBeVisible();
  await expect(comics.locator('.count')).toHaveText('2');

  await page.getByRole('button', { name: 'No comics', exact: true }).click();

  await expect(comics).toBeVisible();
  await expect(comics.locator('.count')).toHaveText('0');
  expect(dedicatedComicCountRequests()).toBe(0);
});
