import { expect, test, type Page, type Route } from '@playwright/test';

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
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

async function signIn(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

async function unloadIsBlocked(page: Page) {
  return page.evaluate(() => {
    const event = new Event('beforeunload', { cancelable: true });
    return { allowed: window.dispatchEvent(event), defaultPrevented: event.defaultPrevented };
  });
}

test('browser unload is guarded only while local upload transfers are active and internal navigation remains available', async ({ page }) => {
  await mockApp(page);

  let pendingUpload: Route | undefined;
  await page.route('**/api/v1/uploads', async (route) => {
    pendingUpload = route;
  });

  await signIn(page);
  await page.locator('input[type="file"]').setInputFiles({
    name: 'guarded.jpg',
    mimeType: 'image/jpeg',
    buffer: Buffer.from('guarded')
  });

  expect(await unloadIsBlocked(page)).toEqual({ allowed: true, defaultPrevented: false });

  await page.getByRole('button', { name: /Upload 1 file/ }).click();
  await expect.poll(() => Boolean(pendingUpload)).toBe(true);
  expect(await unloadIsBlocked(page)).toEqual({ allowed: false, defaultPrevented: true });

  await page.locator('.sidebar button.sidebar-item').filter({ hasText: 'Library' }).click();
  await expect(page.locator('.sidebar-item.active')).toContainText('Library');
  expect(await unloadIsBlocked(page)).toEqual({ allowed: false, defaultPrevented: true });

  await pendingUpload!.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      files: [{ name: 'guarded.jpg', size: 7, target_id: 'default', status: 'imported' }],
      affected_count: 1
    })
  });

  await expect.poll(async () => (await unloadIsBlocked(page)).defaultPrevented).toBe(false);
  expect(await unloadIsBlocked(page)).toEqual({ allowed: true, defaultPrevented: false });
});
