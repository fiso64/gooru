import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function openUpload(page: Page) {
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
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
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

test('drop behavior control lives inside the drop zone without nested interactive semantics', async ({ page }) => {
  await openUpload(page);

  const dropZone = page.locator('.upload-zone');
  const behavior = dropZone.getByLabel('Upload drop behavior');
  const browseSurface = dropZone.getByRole('button', { name: 'Browse files from drop zone' });

  await expect(dropZone).not.toHaveAttribute('role', 'button');
  await expect(dropZone).not.toHaveAttribute('tabindex');
  await expect(browseSurface).toBeVisible();
  await expect(behavior).toBeVisible();
  await expect(behavior.getByRole('button', { name: 'Stage first' })).toBeVisible();
  await expect(behavior.getByRole('button', { name: 'Auto-upload' })).toBeVisible();
  await expect(page.locator('.upload-config-card').getByText('On drop', { exact: true })).toHaveCount(0);

  const geometry = await Promise.all([dropZone.boundingBox(), behavior.boundingBox()]);
  expect(geometry[0]).not.toBeNull();
  expect(geometry[1]).not.toBeNull();
  expect(geometry[1]!.x).toBeGreaterThan(geometry[0]!.x + geometry[0]!.width / 2);
  expect(geometry[1]!.y).toBeGreaterThanOrEqual(geometry[0]!.y);
  expect(geometry[1]!.y).toBeLessThan(geometry[0]!.y + geometry[0]!.height / 2);

  await behavior.getByRole('button', { name: 'Auto-upload' }).click();
  await expect(behavior.getByRole('button', { name: 'Auto-upload' })).toHaveClass(/is-active/);
});
