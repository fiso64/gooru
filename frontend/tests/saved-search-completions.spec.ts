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
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'saved-one', name: 'Favorites', query: 'rating:5', sort: 'created', order: 'desc' }] })
  }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    const q = new URL(route.request().url()).searchParams.get('q') ?? '';
    const negated = q.startsWith('-');
    const items = q.replace(/^-/, '').toLowerCase().startsWith('@saved:')
      ? [
          { name: `${negated ? '-' : ''}@saved:Favorites`, value: 'saved search' },
          { name: `${negated ? '-' : ''}@saved:Family`, value: 'saved search' }
        ]
      : [];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items,
        meta_tags: [
          { name: 'saved', syntax: '@saved:', hint: 'saved search', requires_value: true },
          { name: 'extension', syntax: 'ext:', hint: 'file extension', requires_value: true },
          { name: 'type', syntax: 'type:', hint: 'media type', requires_value: true }
        ]
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

test('saved search names autocomplete after typing @saved: without a redundant prefix or fake counts', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.fill('@saved:');
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Favorites');
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Family');
  await expect.poll(() => visibleCompletions(page)).not.toContain('@saved:');

  const favorites = page.getByRole('option').filter({ hasText: '@saved:Favorites' });
  await expect(favorites.locator('.count')).toHaveCount(0);

  await search.fill('@saved:fav');
  await expect.poll(() => visibleCompletions(page)).toContain('@saved:Favorites');
  await search.press('Enter');
  await expect(page.getByRole('button', { name: 'Remove @saved:Favorites' })).toBeVisible();
});

test('completed value-taking prefixes are not offered as their own completions', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  for (const prefix of ['ext:', 'type:']) {
    await search.fill(prefix);
    await expect(page.getByRole('option', { name: prefix, exact: true })).toHaveCount(0);
  }
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
