import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: string, name: string) {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `library/${name}`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  const files = [fileItem('one', 'one.jpg'), fileItem('two', 'two.jpg')];
  let loggedIn = false;

  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: loggedIn ? 200 : 401,
      contentType: 'application/json',
      body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
    });
  });
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>'
  }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>'
  }));
  await page.route('**/api/v1/files/*/content', async (route) => route.fulfill({ status: 404, contentType: 'text/plain', body: 'not needed' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('Library header keeps sorting separate from selection controls', async ({ page }) => {
  await mockApp(page);
  const actions = page.locator('.library-head-actions');
  await expect(actions.getByRole('button', { name: 'Added' })).toBeVisible();
  await expect(actions.getByRole('button', { name: 'Name' })).toBeVisible();
  await expect(actions.getByRole('button', { name: 'Size' })).toBeVisible();
  await expect(actions.getByText('Select all')).toHaveCount(0);
});

test('ArrowDown in the viewer cannot activate the background media-grid cursor', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  await expect(page.getByRole('dialog', { name: 'one.jpg' })).toBeVisible();

  const stage = page.locator('.viewer-stage');
  await stage.focus();
  await expect(stage).toBeFocused();

  await page.keyboard.press('ArrowDown');
  await expect(stage).toBeFocused();
  await expect(page.locator('.thumb-open:focus')).toHaveCount(0);

  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
});
