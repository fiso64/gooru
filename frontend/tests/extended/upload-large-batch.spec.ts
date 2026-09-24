import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function openUpload(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: loggedIn ? 200 : 401, contentType: 'application/json', body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } }) }));
  await page.route('**/api/v1/auth/login', async (route) => { loggedIn = true; await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }); });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

test('dropping ten thousand images keeps rendered rows and blob previews bounded', async ({ page }) => {
  await page.addInitScript(() => {
    const originalCreate = URL.createObjectURL.bind(URL);
    const originalRevoke = URL.revokeObjectURL.bind(URL);
    const stats = { active: 0, peak: 0, created: 0 };
    Object.defineProperty(window, '__uploadBlobStats', { value: stats, configurable: false });
    URL.createObjectURL = (object: Blob | MediaSource) => { stats.active += 1; stats.created += 1; stats.peak = Math.max(stats.peak, stats.active); return originalCreate(object); };
    URL.revokeObjectURL = (url: string) => { stats.active = Math.max(0, stats.active - 1); originalRevoke(url); };
  });
  await openUpload(page);

  await page.getByRole('group', { name: 'File upload drop zone' }).evaluate((zone) => {
    const transfer = new DataTransfer();
    for (let index = 0; index < 10_000; index += 1) {
      transfer.items.add(new File([new Uint8Array([index % 251])], `geom_${String(index + 1).padStart(5, '0')}.jpg`, { type: 'image/jpeg' }));
    }
    zone.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer: transfer }));
  });

  await expect(page.getByText(/Staged · 10000 files/)).toBeVisible({ timeout: 15_000 });
  const list = page.getByTestId('staged-upload-list');
  const pager = page.getByRole('navigation', { name: 'Staged upload pages' });
  await expect(list.locator('.upload-row')).toHaveCount(100);
  await expect(pager.getByRole('button', { name: 'Page 1', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(pager.getByRole('button', { name: 'Page 100', exact: true })).toBeVisible();
  await expect(list.getByText('geom_00001.jpg')).toBeVisible();
  await expect(list.getByText('geom_00101.jpg')).toHaveCount(0);

  const stats = await page.evaluate(() => (window as unknown as { __uploadBlobStats: { active: number; peak: number; created: number } }).__uploadBlobStats);
  expect(stats.peak).toBeLessThan(30);
  expect(stats.created).toBeLessThan(50);

  await pager.getByRole('button', { name: 'Next page' }).click();
  await expect(pager.getByRole('button', { name: 'Page 2', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(list.getByText('geom_00101.jpg')).toBeVisible();
  await expect(list.locator('.upload-row')).toHaveCount(100);

  await pager.getByRole('button', { name: 'Page 100', exact: true }).click();
  await expect(pager.getByRole('button', { name: 'Page 100', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(list.getByText('geom_09901.jpg')).toBeVisible();
  await expect(list.locator('.upload-row')).toHaveCount(100);
});
