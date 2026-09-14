import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const completionTags = [
  { name: 'series:superhero', namespace: 'series', value: 'superhero', count: 100 },
  { name: 'series:my_hero_academia', namespace: 'series', value: 'my_hero_academia', count: 3 },
  { name: 'heroic', count: 7 },
  { name: 'subject:hero', namespace: 'subject', value: 'hero', count: 5 },
  { name: 'character:alice', namespace: 'character', value: 'alice', count: 100 },
  { name: 'technology', count: 2 }
];

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
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: completionTags }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
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

async function visibleTagOrder(page: Page): Promise<string[]> {
  return page.locator('#searchbar-suggestions [role="option"] .tok').allTextContents();
}

test('library search ranks component-prefix tags ahead of higher-use substring matches', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.pressSequentially('hero', { delay: 10 });
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  await expect.poll(() => visibleTagOrder(page)).toEqual([
    'heroic',
    'subject:hero',
    'series:my_hero_academia',
    'series:superhero'
  ]);
});

test('namespace value completions use the same component-prefix ranking', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.pressSequentially('series:hero', { delay: 10 });
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  await expect.poll(() => visibleTagOrder(page)).toEqual([
    'series:my_hero_academia',
    'series:superhero'
  ]);
});

test('bare search uses the shared policy across tag and namespace candidates', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.pressSequentially('te', { delay: 10 });
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  await expect(page.locator('#searchbar-suggestions [role="option"]').first()).toContainText('technology');
});


test('rapid completion typing keeps the popup stable and request responses bound to their exact query', async ({ page }) => {
  await mockApp(page);
  const seenQueries: string[] = [];
  await page.unroute('**/api/v1/search/suggestions?**');
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    const q = new URL(route.request().url()).searchParams.get('q') ?? '';
    seenQueries.push(q);
    await new Promise((resolve) => setTimeout(resolve, 75));
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        items: [],
        meta_tags: [{ syntax: '@filename_contains:', hint: 'filename contains', requires_value: true }]
      })
    });
  });

  const search = page.getByLabel('Search library');
  await search.fill('@f');
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
  for (const suffix of ['i', 'l', 'e', 'n', 'a', 'm', 'e']) {
    await search.press(suffix);
    await expect(page.getByRole('listbox', { name: 'Search suggestions' })).toBeVisible();
    await expect(page.locator('#searchbar-suggestions [role="option"]').first()).toContainText('@filename_contains');
  }
  await expect.poll(() => seenQueries.at(-1)).toBe('@filename');
  expect(seenQueries.every((query) => '@filename'.startsWith(query))).toBe(true);
});

test('search suggestions omit redundant group headers and keyboard footer', async ({ page }) => {
  await mockApp(page);
  const search = page.getByLabel('Search library');

  await search.pressSequentially('hero', { delay: 10 });
  const suggestions = page.getByRole('listbox', { name: 'Search suggestions' });
  await expect(suggestions).toBeVisible();
  await expect(suggestions.getByRole('option')).not.toHaveCount(0);
  await expect(suggestions.getByText('Suggestions', { exact: true })).toHaveCount(0);
  await expect(suggestions.getByText('Query', { exact: true })).toHaveCount(0);
  await expect(suggestions.getByText(/navigate|select|close|prefix .* to exclude/i)).toHaveCount(0);
});
