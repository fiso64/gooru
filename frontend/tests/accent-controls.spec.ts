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
  await page.route('**/api/v1/file-selections', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'selection-accent', count: files.length }) });
  });
  await page.route('**/api/v1/file-selections/*/members', async (route) => {
    const body = route.request().postDataJSON() as { file_ids?: string[] };
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ file_ids: body.file_ids ?? [] }) });
  });
  await page.route('**/api/v1/file-selections/*', async (route) => {
    if (route.request().method() === 'DELETE') {
      await route.fulfill({ status: 204 });
      return;
    }
    await route.fallback();
  });
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
    const underline = node.querySelector(':scope > u');
    if (!(underline instanceof HTMLElement)) throw new Error('shortcut label is missing a direct underline');

    let tail = underline.nextSibling;
    while (tail && !(tail instanceof Text)) tail = tail.nextSibling;
    if (!(tail instanceof Text) || tail.length === 0) throw new Error('shortcut label is missing trailing text');

    const firstVisibleCharacter = tail.data.search(/\S/);
    if (firstVisibleCharacter < 0) throw new Error('shortcut label trailing text is blank');
    const tailRange = document.createRange();
    tailRange.setStart(tail, firstVisibleCharacter);
    tailRange.setEnd(tail, firstVisibleCharacter + 1);
    return tailRange.getBoundingClientRect().left - underline.getBoundingClientRect().right;
  });
}

test('file-selection checkbox follows the configured accent', async ({ page }) => {
  await mockLibrary(page);

  await page.locator('.gooru-root').evaluate((node) => {
    (node as HTMLElement).style.setProperty('--accent', 'rgb(12, 34, 56)');
  });

  await page.keyboard.press('a');
  const selected = page.getByRole('checkbox', { name: 'Deselect one.jpg' });
  await expect(selected).toBeVisible();
  await expect(selected).toHaveCSS('background-color', 'rgb(12, 34, 56)');
  await expect(selected).toHaveCSS('border-color', 'rgb(12, 34, 56)');
});

test('shortcut underlines do not create visual spaces inside button labels', async ({ page }) => {
  await mockLibrary(page);

  await page.keyboard.press('a');
  await expect(page.locator('.selection-bar')).toBeVisible();

  const shortcutButtons = page.locator('.selection-bar button.g-btn:has(> u)');
  // Every selection action with an underlined shortcut must keep its label tight.
  await expect(shortcutButtons).toHaveCount(3);
  for (const button of await shortcutButtons.all()) {
    expect(await gapAfterShortcut(button)).toBeLessThanOrEqual(1);
  }
});
