import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page, theme: 'booru-light' | 'booru-dark') {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ ui_theme: theme })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [], library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [], meta_tags: [] })
  }));
  await page.route('**/api/v1/operations?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      items: [{
        id: 'op-1',
        kind: 'delete_files',
        status: 'running',
        progress_total: 2,
        progress_completed: 1,
        progress_failed: 0,
        created_at: '2026-09-17T20:00:00Z'
      }],
      total_count: 1,
      active_count: 1
    })
  }));
}

for (const theme of ['booru-light', 'booru-dark'] as const) {
  test(`${theme} keeps real progress bars square`, async ({ page }) => {
    await mockApp(page, theme);
    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

    await page.locator('.gooru-root').evaluate((root) => {
      const native = document.createElement('progress');
      native.dataset.testid = 'native-progress';
      native.max = 100;
      native.value = 50;
      root.append(native);

      for (const className of ['progress', 'video-progress']) {
        const bar = document.createElement('div');
        bar.className = className;
        bar.dataset.testid = className;
        root.append(bar);
      }
    });

    for (const testID of ['native-progress', 'progress', 'video-progress']) {
      await expect(page.getByTestId(testID)).toHaveCSS('border-radius', '0px');
    }

    await page.getByRole('button', { name: 'Jobs drawer' }).click();
    await expect(page.locator('#jobs-drawer .job-progress')).toHaveCSS('border-radius', '0px');

    await page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('button', { name: 'Jobs', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Background work' })).toBeVisible();
    await expect(page.locator('.jobs-page .job-progress')).toHaveCSS('border-radius', '0px');
  });
}
