import { expect, test, type Locator, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockLibrary(page: Page) {
  let loggedIn = false;
  const files = [{
    id: 'one',
    content_id: 'hash-one',
    name: 'one.jpg',
    safe_display_path: 'library/one.jpg',
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 },
    tags: [],
    media_urls: {
      thumbnail: '/api/v1/files/one/thumbnail',
      preview: '/api/v1/files/one/preview',
      content: '/api/v1/files/one/content',
      download: '/api/v1/files/one/download'
    }
  }];

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [], library_count: 1, facets: { kind: [] } }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function gapAfterShortcut(locator: Locator) {
  return locator.evaluate((node) => {
    const underline = node.querySelector('u');
    const tail = underline?.nextSibling;
    if (!(underline instanceof HTMLElement) || !(tail instanceof Text)) throw new Error('shortcut label is missing an inline trailing text node');
    const tailRange = document.createRange();
    tailRange.setStart(tail, 0);
    tailRange.setEnd(tail, Math.min(1, tail.length));
    return tailRange.getBoundingClientRect().left - underline.getBoundingClientRect().right;
  });
}

test('select-all checkbox follows the configured accent', async ({ page }) => {
  await mockLibrary(page);

  await page.locator('.gooru-root').evaluate((node) => {
    (node as HTMLElement).style.setProperty('--accent', 'rgb(12, 34, 56)');
  });

  const selectAll = page.getByLabel('Select all files in current view');
  await expect(selectAll).toHaveCSS('accent-color', 'rgb(12, 34, 56)');

  await selectAll.click();
  await expect(selectAll).toBeChecked();
  await expect(selectAll).toHaveCSS('accent-color', 'rgb(12, 34, 56)');
});

test('shortcut underlines do not create visual spaces inside button labels', async ({ page }) => {
  await mockLibrary(page);

  const selectAll = page.getByLabel('Select all files in current view');
  const selectAllLabel = selectAll.locator('..');
  expect(await gapAfterShortcut(selectAllLabel)).toBeLessThanOrEqual(1);

  await selectAll.click();
  await expect(page.getByText('1 of 1 selected')).toBeVisible();

  const tag = page.getByRole('button', { name: 'Tag…' });
  const untag = page.getByRole('button', { name: 'Untag…' });
  expect(await gapAfterShortcut(tag)).toBeLessThanOrEqual(1);
  expect(await gapAfterShortcut(untag)).toBeLessThanOrEqual(1);
});
