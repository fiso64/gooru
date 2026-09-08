import { expect, test, type Page } from '@playwright/test';

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
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'saved-one', name: 'Favorites', query: 'rating:5', sort: 'created', order: 'desc' }] })
  }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    const q = new URL(route.request().url()).searchParams.get('q') ?? '';
    const items = q.toLowerCase().startsWith('@saved:')
      ? [{ name: '@saved:Favorites', value: 'saved search' }, { name: '@saved:Family', value: 'saved search' }]
      : [];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items,
        meta_tags: [{ name: 'saved', syntax: '@saved:', hint: 'saved search', requires_value: true }]
      })
    });
  });
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function visibleCompletions(page: Page): Promise<string[]> {
  return page.locator('#searchbar-suggestions [role="option"] .tok').allTextContents();
}

test('saved search names autocomplete after typing @saved:', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.fill('@saved:');
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Favorites');
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Family');

  await search.fill('@saved:fav');
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Favorites');
  await search.press('Enter');
  await expect(page.getByRole('button', { name: 'Remove @saved:Favorites' })).toBeVisible();
});

test('negated saved search names autocomplete without duplicating the negation prefix', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.fill('-@saved:fam');
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Family');
  await search.press('Enter');
  await expect(page.getByRole('button', { name: 'Remove -@saved:Family' })).toBeVisible();
});
