import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page, onFilesRequest: () => void) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => {
    onFilesRequest();
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      tags: [],
      library_count: 7,
      facets: { kind: [{ value: 'photo', count: 4 }, { value: 'video', count: 2 }, { value: 'gif', count: 1 }] }
    })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

test('non-library route does not materialize the files grid and library still loads it on demand', async ({ page }) => {
  let filesRequests = 0;
  await mockApp(page, () => filesRequests += 1);

  await page.goto('/tags');
  await expect(page.getByRole('heading', { name: 'Tags' })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Library 7$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Photos 4$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /^Videos 2$/ })).toBeVisible();
  await expect(page.getByRole('button', { name: /^GIFs 1$/ })).toBeVisible();
  await page.waitForTimeout(250);
  expect(filesRequests).toBe(0);

  await page.getByRole('button', { name: /^Library \d+$/ }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect.poll(() => filesRequests).toBeGreaterThan(0);
});
