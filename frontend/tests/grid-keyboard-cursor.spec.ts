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
    tags: id === 'one' ? ['alpha'] : [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  const files = [fileItem('one', 'one.jpg'), fileItem('two', 'two.jpg'), fileItem('three', 'three.jpg')];
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
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      tags: [
        { name: 'alpha', value: 'alpha', namespace: '', count: 3 },
        { name: 'beta', value: 'beta', namespace: '', count: 2 },
        { name: 'subject:cat', value: 'cat', namespace: 'subject', count: 1 }
      ]
    })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#fff"/></svg>' }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('library viewport keeps initial keyboard focus without drawing a Chromium focus ring', async ({ page }) => {
  await mockApp(page);

  const viewport = page.getByTestId('library-viewport');
  await expect(viewport).toBeFocused();
  const focusStyle = await viewport.evaluate((node) => {
    const style = getComputedStyle(node);
    return { outlineStyle: style.outlineStyle, outlineWidth: style.outlineWidth };
  });
  expect(focusStyle.outlineStyle).toBe('none');

  await page.keyboard.press('ArrowDown');
  await expect(page.locator('.thumb-open').nth(0)).toBeFocused();
});

test('ArrowDown enters the media grid with a high-contrast cursor and cursor actions select, move, and open', async ({ page }) => {
  await mockApp(page);

  const search = page.getByLabel('Search library');
  await search.fill('alpha');
  await search.press('Enter');
  await expect(search).toBeFocused();
  await expect(search).toHaveValue('');
  await search.press('ArrowDown');

  const cards = page.locator('.thumb-open');
  await expect(cards.nth(0)).toBeFocused();
  await expect(cards.nth(0)).toHaveAccessibleName('Preview one.jpg');
  const cursor = await cards.nth(0).evaluate((node) => {
    const style = getComputedStyle(node, '::after');
    return {
      content: style.content,
      borderStyle: style.borderStyle,
      borderColor: style.borderColor,
      boxShadow: style.boxShadow
    };
  });
  expect(cursor.content).not.toBe('none');
  expect(cursor.borderStyle).toBe('dashed');
  expect(cursor.borderColor).toBe('rgb(255, 255, 255)');
  expect(cursor.boxShadow).toContain('rgb(0, 0, 0)');

  await page.keyboard.press('Space');
  await expect(page.getByText('1 selected', { exact: true })).toBeVisible();
  await expect(cards.nth(0)).toBeFocused();

  await page.keyboard.press('ArrowRight');
  await expect(cards.nth(1)).toBeFocused();

  await page.keyboard.press('Enter');
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
});

test('modified arrows start, extend, and shrink the cursor selection range', async ({ page }) => {
  await mockApp(page);
  const cards = page.locator('.thumb-open');

  await page.keyboard.press('ArrowDown');
  await expect(cards.nth(0)).toBeFocused();

  await page.keyboard.press('Shift+ArrowRight');
  await expect(cards.nth(1)).toBeFocused();
  await expect(page.getByText('2 selected', { exact: true })).toBeVisible();

  await page.keyboard.press('Shift+ArrowRight');
  await expect(cards.nth(2)).toBeFocused();
  await expect(page.getByText('3 selected', { exact: true })).toBeVisible();

  await page.keyboard.press('Shift+ArrowLeft');
  await expect(cards.nth(1)).toBeFocused();
  await expect(page.getByText('2 selected', { exact: true })).toBeVisible();

  await page.keyboard.press('Shift+ArrowLeft');
  await expect(cards.nth(0)).toBeFocused();
  await expect(page.getByText('1 selected', { exact: true })).toBeVisible();
});


test('ArrowDown enters the tags grid without stealing arrows from the filter', async ({ page }) => {
  await mockApp(page);

  await page.locator('.sidebar').getByRole('button', { name: /^Tags\b/ }).click();
  await expect(page.getByRole('heading', { name: /tags across/i })).toBeVisible();

  const filter = page.getByPlaceholder('Filter tags…');
  await filter.fill('a');
  await filter.press('ArrowDown');
  await expect(filter).toBeFocused();

  await page.getByRole('heading', { name: /tags across/i }).click();
  await page.keyboard.press('ArrowDown');

  const tagItems = page.locator('.tagscloud-item:not(.skeleton)');
  await expect(tagItems.nth(0)).toBeFocused();
  await page.keyboard.press('ArrowRight');
  await expect(tagItems.nth(1)).toBeFocused();
});

test('Escape hides the grid keyboard cursor when no higher-priority exit is active', async ({ page }) => {
  await mockApp(page);
  await page.keyboard.press('ArrowDown');
  const first = page.locator('.thumb-open').first();
  await expect(first).toBeFocused();

  await page.keyboard.press('Escape');
  await expect(first).not.toBeFocused();
});


test('cursor tag and removal shortcuts target the focused file while selection keeps precedence', async ({ page }) => {
  await mockApp(page);
  const cards = page.locator('.thumb-open');

  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('u');
  let dialog = page.getByRole('dialog', { name: 'Edit tags · one.jpg' });
  await expect(dialog.getByRole('textbox', { name: 'Remove tags from one.jpg' })).toBeFocused();
  await dialog.getByRole('button', { name: 'Cancel' }).click();

  await page.keyboard.press('Alt+Enter');
  dialog = page.getByRole('dialog', { name: 'Edit tags · one.jpg' });
  await expect(dialog.getByRole('textbox', { name: 'Tags for one.jpg' })).toBeFocused();
  await dialog.getByRole('button', { name: 'Cancel' }).click();

  await page.keyboard.press('Delete');
  dialog = page.getByRole('dialog', { name: 'Remove from library' });
  await expect(dialog).toContainText('one.jpg');
  await dialog.getByRole('button', { name: 'Cancel' }).click();

  await page.keyboard.press('Shift+Delete');
  dialog = page.getByRole('dialog', { name: 'Delete file' });
  await expect(dialog).toContainText('one.jpg');
  await dialog.getByRole('button', { name: 'Cancel' }).click();

  await page.keyboard.press('Space');
  await expect(page.getByText('1 selected', { exact: true })).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await expect(cards.nth(1)).toBeFocused();
  await page.keyboard.press('t');
  await expect(page.getByRole('dialog', { name: 'Edit tags · one.jpg' })).toBeVisible();
});


test('single-file tag editor stages changes until Apply', async ({ page }) => {
  const tagRequests: Array<{ method: string; body: Record<string, unknown> }> = [];
  await page.route('**/api/v1/files/tags', async (route) => {
    tagRequests.push({ method: route.request().method(), body: route.request().postDataJSON() });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ updated_files: 1 }) });
  });
  await mockApp(page);

  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('t');
  const dialog = page.getByRole('dialog', { name: 'Edit tags · one.jpg' });
  await expect(dialog.getByText('alpha', { exact: true })).toBeVisible();

  const input = dialog.getByRole('textbox', { name: 'Tags for one.jpg' });
  await input.fill('beta');
  await input.press('Enter');
  expect(tagRequests).toHaveLength(0);

  await dialog.getByRole('button', { name: 'Apply' }).click();
  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0].method).toBe('POST');
  expect(tagRequests[0].body).toMatchObject({ file_ids: ['one'], tags: ['beta'] });
  await expect(page.locator('.thumb-open').nth(0)).toBeFocused();

  await page.keyboard.press('u');
  const removeDialog = page.getByRole('dialog', { name: 'Edit tags · one.jpg' });
  const removeInput = removeDialog.getByRole('textbox', { name: 'Remove tags from one.jpg' });
  await removeInput.fill('alpha');
  await removeInput.press('Enter');
  expect(tagRequests).toHaveLength(1);

  await removeDialog.getByRole('button', { name: 'Apply' }).click();
  await expect.poll(() => tagRequests.length).toBe(2);
  expect(tagRequests[1].method).toBe('DELETE');
  expect(tagRequests[1].body).toMatchObject({ file_ids: ['one'], tags: ['alpha'] });
});


test('cursor download sends a single-file selector', async ({ page }) => {
  const requests: Array<Record<string, unknown>> = [];
  await page.route('**/api/v1/file-downloads', async (route) => {
    requests.push(route.request().postDataJSON());
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'dl-one', url: '/api/v1/file-downloads/dl-one' }) });
  });
  await page.route('**/api/v1/file-downloads/*', async (route) => route.fulfill({ status: 204 }));
  await mockApp(page);

  await page.keyboard.press('ArrowDown');
  await page.keyboard.press('d');
  await expect.poll(() => requests.length).toBe(1);
  expect(requests[0]).toEqual({ file_ids: ['one'] });
});
