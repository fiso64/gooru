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
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  let uploadIndex = 0;
  await page.route('**/api/v1/uploads', async (route) => {
    uploadIndex += 1;
    await route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify({ id: `job-${uploadIndex}`, kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-09-02T00:00:00Z' }) });
  });
  await page.route('**/api/v1/operations/job-*', async (route) => {
    const id = route.request().url().split('/').at(-1) ?? '';
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id, kind: 'upload_import', status: 'pending', progress_total: 1, progress_completed: 0, progress_failed: 0, created_at: '2026-09-02T00:00:00Z' }) });
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

test('long ASCII and CJK filenames cannot displace upload size progress or status columns', async ({ page }) => {
  await mockApp(page);
  const chooser = page.locator('input[type="file"]');
  const asciiName = `${'very-long-file-name-'.repeat(14)}.jpg`;
  const cjkName = `${'東京写真資料集'.repeat(22)}.jpg`;
  await chooser.setInputFiles([
    { name: asciiName, mimeType: 'image/jpeg', buffer: Buffer.from('a') },
    { name: cjkName, mimeType: 'image/jpeg', buffer: Buffer.from('b') }
  ]);
  await page.getByRole('button', { name: /Upload 2 files/ }).click();

  const rows = page.locator('.upload-row');
  await expect(rows).toHaveCount(2);
  await expect(rows.nth(0).locator('.status')).toHaveText(/queued|importing/);
  await expect(rows.nth(1).locator('.status')).toHaveText(/queued|importing/);

  const geometry = await rows.evaluateAll((nodes) => nodes.map((node) => {
    const row = node.getBoundingClientRect();
    const name = node.querySelector<HTMLElement>('.name')!;
    const size = node.querySelector<HTMLElement>('.size')!.getBoundingClientRect();
    const progress = node.querySelector<HTMLElement>('.progress')!.getBoundingClientRect();
    const status = node.querySelector<HTMLElement>('.status')!.getBoundingClientRect();
    return {
      rowRight: row.right,
      nameClientWidth: name.clientWidth,
      nameScrollWidth: name.scrollWidth,
      sizeX: size.x,
      sizeRight: size.right,
      progressX: progress.x,
      progressWidth: progress.width,
      statusX: status.x,
      statusRight: status.right
    };
  }));

  for (const item of geometry) {
    expect(item.nameScrollWidth).toBeGreaterThan(item.nameClientWidth);
    expect(item.statusRight).toBeLessThanOrEqual(item.rowRight + 0.5);
    expect(item.progressWidth).toBeGreaterThan(100);
    expect(item.sizeRight).toBeLessThan(item.progressX);
    expect(item.progressX + item.progressWidth).toBeLessThan(item.statusX);
  }
  expect(Math.abs(geometry[0].sizeX - geometry[1].sizeX)).toBeLessThan(0.5);
  expect(Math.abs(geometry[0].progressX - geometry[1].progressX)).toBeLessThan(0.5);
  expect(Math.abs(geometry[0].statusX - geometry[1].statusX)).toBeLessThan(0.5);
});
