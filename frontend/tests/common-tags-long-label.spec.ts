import { expect, test } from '@playwright/test';

test('long common tag labels ellipsize without shrinking the tag icon', async ({ page }) => {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ user: { id: 'usr_test', username: 'mac', role: 'admin' }, capabilities: { upload: true, tag: true, delete: true, admin: true }, csrf_token: 'csrf-one' }) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [{ name: 'artist:this-is-an-extremely-long-common-tag-label-that-must-ellipsis', count: 100 }, { name: 'short', count: 50 }], library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));

  await page.goto('/');
  const row = page.locator('.common-tag-item').first();
  const icon = row.locator('svg').first();
  const label = row.locator('.truncate');

  await expect(row).toBeVisible();
  await expect(icon).toHaveCSS('width', '14px');
  await expect(icon).toHaveCSS('height', '14px');
  expect(await icon.evaluate((node) => getComputedStyle(node).flexShrink)).toBe('0');
  expect(await label.evaluate((node) => node.scrollWidth > node.clientWidth)).toBe(true);
  await expect(label).toHaveCSS('text-overflow', 'ellipsis');
});
