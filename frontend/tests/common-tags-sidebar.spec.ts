import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page, fileQueries: string[]) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => {
    fileQueries.push(new URL(route.request().url()).searchParams.get('query') ?? '');
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
    });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'saved-1', name: 'Favorites', query: 'rating:5', sort: 'name', order: 'asc' }] })
  }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => {
    const tags = [
      ...Array.from({ length: 24 }, (_, index) => ({ name: `tag-${String(index + 1).padStart(2, '0')}`, count: index + 1 })),
      { namespace: 'artist', value: 'alice', count: 99 }
    ].reverse();
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ tags, library_count: 0, facets: { kind: [] } })
    });
  });
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

test('shows the 20 most common tags after saved searches and runs a tag search', async ({ page }) => {
  const fileQueries: string[] = [];
  await mockApp(page, fileQueries);
  await page.goto('/');

  const common = page.locator('.common-tags-section');
  await expect(common.getByText('Common tags')).toBeVisible();
  await expect(page.locator('.saved-searches-section + .common-tags-section')).toHaveCount(1);
  await expect(common.locator('button.sidebar-item')).toHaveCount(20);
  await expect(common.locator('button.sidebar-item').first()).toContainText('artist:alice');
  await expect(common.locator('button.sidebar-item').first()).toContainText('99');
  await expect(common.getByRole('button', { name: /tag-24 24/ })).toBeVisible();
  await expect(common.getByRole('button', { name: /tag-06 6/ })).toBeVisible();
  await expect(common.getByText('tag-05', { exact: true })).toHaveCount(0);

  await common.locator('button.sidebar-item').first().click();
  await expect.poll(() => fileQueries.includes('artist:alice')).toBe(true);
});
