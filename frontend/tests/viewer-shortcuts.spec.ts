import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const file = {
  id: 'one',
  content_id: 'hash-one',
  name: 'one.jpg',
  safe_display_path: 'library/one.jpg',
  size: 2048,
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'image/jpeg',
  media_kind: 'photo',
  metadata: { image_width: 800, image_height: 600 },
  tags: ['alpha', 'project:blue'],
  can_delete: true,
  media_urls: {
    thumbnail: '/api/v1/files/one/thumbnail',
    preview: '/api/v1/files/one/preview',
    content: '/api/v1/files/one/content',
    download: '/api/v1/files/one/download'
  }
};

type TagRequest = { operation: string; body: { file_ids?: string[]; tags?: string[] } };

async function mockApp(page: Page) {
  let loggedIn = false;
  const tagRequests: TagRequest[] = [];

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ font_style: 'editorial', load_full_media_by_default: false })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [
      { name: 'alpha', count: 3 },
      { name: 'project:blue', count: 2 },
      { name: 'gamma', count: 1 }
    ] })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
  }));
  await page.route('**/api/v1/files/tags', async (route) => {
    const method = route.request().method();
    if (!['POST', 'PUT', 'DELETE'].includes(method)) return route.fallback();
    tagRequests.push({
      operation: method === 'POST' ? 'add' : method === 'PUT' ? 'set' : 'remove',
      body: route.request().postDataJSON()
    });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ updated_files: 1 }) });
  });
  await page.route('**/api/v1/files/one/thumbnail**', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" />' }));
  await page.route('**/api/v1/files/one/preview**', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" />' }));
  await page.context().route('**/api/v1/files/one/content', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" />' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return tagRequests;
}

test('viewer shortcuts switch tag modes and dispatch file actions', async ({ page }) => {
  const tagRequests = await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const preview = page.getByRole('dialog', { name: 'one.jpg' });
  await expect(preview).toBeVisible();

  await page.keyboard.press('?');
  const shortcuts = page.getByRole('dialog', { name: 'Shortcuts' });
  await expect(shortcuts).toBeVisible();
  await expect(preview).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(shortcuts).toHaveCount(0);
  await expect(preview).toBeVisible();

  await page.keyboard.press('u');
  const untagInput = page.getByRole('textbox', { name: 'Remove tags from one.jpg' });
  await expect(untagInput).toBeFocused();
  await untagInput.fill('a');
  await expect(page.getByRole('option', { name: /alpha/ })).toBeVisible();
  await expect(page.getByRole('option', { name: /gamma/ })).toHaveCount(0);
  await untagInput.press('Enter');
  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0]).toMatchObject({ operation: 'remove', body: { file_ids: ['one'], tags: ['alpha'] } });

  await preview.focus();
  await page.keyboard.press('t');
  await expect(page.getByRole('textbox', { name: 'Tags for one.jpg' })).toBeFocused();

  await preview.focus();
  const originalToggle = preview.getByRole('button', { name: 'Use original media' });
  await expect(originalToggle).toHaveAttribute('title', 'Use original media (Q)');
  await page.keyboard.press('q');
  const previewToggle = preview.getByRole('button', { name: 'Use derived preview' });
  await expect(previewToggle).toHaveAttribute('aria-pressed', 'true');
  await expect(previewToggle).toHaveAttribute('title', 'Use derived preview (Q)');
  await expect(preview.getByTitle('Add tag (T)')).toBeVisible();
  await expect(preview.getByTitle('Download original (D)')).toBeVisible();
  await expect(preview.getByTitle('Delete file from disk (Shift+Delete)')).toBeVisible();
  await expect(preview.getByTitle('Remove from library without deleting the file (Delete)')).toBeVisible();

  await preview.focus();
  await page.keyboard.press('Delete');
  await expect(page.getByRole('dialog', { name: 'Remove from library' })).toBeVisible();
  await page.keyboard.press('Escape');

  await preview.focus();
  await page.keyboard.press('Shift+Delete');
  await expect(page.getByRole('dialog', { name: 'Delete file' })).toBeVisible();
});

test('O opens the currently viewed original in a new browser tab', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const preview = page.getByRole('dialog', { name: 'one.jpg' });
  await expect(preview.getByTitle('Open original in new tab (O)')).toBeVisible();

  const openedPromise = page.context().waitForEvent('page');
  await preview.focus();
  await page.keyboard.press('o');
  const opened = await openedPromise;
  await expect.poll(() => {
    try { return new URL(opened.url()).pathname; } catch { return ''; }
  }).toBe('/api/v1/files/one/content');
  await opened.close();
});

test('shortcuts open as a modal and number keys follow visible sidebar order', async ({ page }) => {
  await mockApp(page);

  await page.keyboard.press('?');
  const shortcuts = page.getByRole('dialog', { name: 'Shortcuts' });
  await expect(shortcuts).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(shortcuts.getByText('Play media or enter / exit comic')).toHaveCount(0);
  await expect(shortcuts.getByText('Clear selection')).toHaveCount(0);
  await expect(shortcuts.getByText('Keys are ignored while you are typing unless the shortcut belongs to that input')).toHaveCount(0);
  await expect(shortcuts.getByText('Focus search')).toBeVisible();
  await expect(shortcuts.getByText('Save current search')).toBeVisible();
  await expect(shortcuts.getByText('Toggle original / preview media')).toBeVisible();
  await expect(shortcuts.getByText('Open original in new tab')).toBeVisible();
  await expect(shortcuts.getByRole('heading', { name: 'Shortcuts' })).toHaveCSS('font-size', '22px');

  const shortcutColumns = shortcuts.locator('.shortcut-column');
  await expect(shortcutColumns).toHaveCount(2);
  await expect(shortcutColumns.nth(0).getByRole('heading', { name: 'Navigation' })).toBeVisible();
  await expect(shortcutColumns.nth(0).getByRole('heading', { name: 'Selection' })).toBeVisible();
  await expect(shortcutColumns.nth(0).getByRole('heading', { name: 'Viewer' })).toHaveCount(0);
  await expect(shortcutColumns.nth(1).getByRole('heading', { name: 'Viewer' })).toBeVisible();

  await page.keyboard.press('Escape');
  await expect(shortcuts).toHaveCount(0);

  const search = page.getByRole('textbox', { name: 'Search library' });
  const searchShortcut = page.locator('.searchbar-shortcut');
  await expect(searchShortcut).toHaveText('/');
  await expect(searchShortcut).toHaveCSS('font-size', '9px');
  await expect(searchShortcut).toHaveCSS('margin-right', '5px');
  const [shortcutColor, subduedColor] = await searchShortcut.evaluate((element) => {
    const root = element.closest('.gooru-root');
    if (!root) throw new Error('search shortcut is outside the themed root');
    const probe = document.createElement('span');
    probe.style.color = 'var(--text-3)';
    root.appendChild(probe);
    const colors = [getComputedStyle(element).color, getComputedStyle(probe).color];
    probe.remove();
    return colors;
  });
  expect(shortcutColor).toBe(subduedColor);
  await expect(page.getByRole('button', { name: 'Clear search' })).toHaveCount(0);
  await page.keyboard.press('/');
  await expect(search).toBeFocused();
  await search.fill('a');
  await expect(searchShortcut).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Clear search' })).toBeVisible();
  await page.getByRole('button', { name: 'Clear search' }).click();
  await expect(searchShortcut).toHaveText('/');
  await search.press('Escape');

  await page.keyboard.press('b');
  await expect(page.getByRole('dialog', { name: 'Save search' })).toBeVisible();
  await page.keyboard.press('Escape');

  await page.keyboard.press('2');
  await expect(page.getByRole('heading', { name: '3 tags across 1 files' })).toBeVisible();

  await page.keyboard.press('1');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
});
