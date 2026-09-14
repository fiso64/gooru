import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

type TagRequest = { method: string; body: { file_ids?: string[]; tags?: string[]; verbose?: boolean } };

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
    tags: ['blue'],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  let loggedIn = false;
  const tagRequests: TagRequest[] = [];
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({})
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
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [
      { name: 'rating:safe', namespace: 'rating', value: 'safe', count: 3 },
      { name: 'blue', count: 2 },
      { name: 'character:alice', namespace: 'character', value: 'alice', count: 100 },
      { name: 'technology', count: 2 }
    ] })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [fileItem('one', 'one.jpg')], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
  }));
  await page.route('**/api/v1/files/tags', async (route) => {
    if (!['POST', 'PUT', 'DELETE'].includes(route.request().method())) return route.fallback();
    tagRequests.push({ method: route.request().method(), body: route.request().postDataJSON() });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ updated_files: 1 }) });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />'
  }));
  return tagRequests;
}

async function openSelectedLibrary(page: Page) {
  const tagRequests = await mockApp(page);
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('checkbox', { name: 'Select one.jpg' }).click();
  return tagRequests;
}

test('Tag selected exposes existing tag completions without covering dialog actions', async ({ page }) => {
  await openSelectedLibrary(page);

  await page.getByRole('button', { name: 'Tag…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  await dialog.getByLabel('Tags').fill('rat');
  const suggestions = dialog.getByRole('listbox', { name: 'Tags suggestions' });
  await expect(suggestions).toBeVisible();
  await expect(suggestions.getByRole('option', { name: /rating:safe/ })).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).click();
  await expect(dialog).toBeHidden();
});

test('Tag selected uses the same prefix-first ordering as main search', async ({ page }) => {
  await openSelectedLibrary(page);

  await page.getByRole('button', { name: 'Tag…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  await dialog.getByLabel('Tags').fill('te');
  const suggestions = dialog.getByRole('listbox', { name: 'Tags suggestions' });
  await expect(suggestions).toBeVisible();
  await expect(suggestions.getByRole('option').first()).toContainText('technology');
});

test('Untag selected exposes existing tag completions', async ({ page }) => {
  await openSelectedLibrary(page);

  await page.locator('.sb-actions button').filter({ hasText: 'Untag…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Untag selected files' });
  await dialog.getByLabel('Tags').fill('bl');
  const suggestions = dialog.getByRole('listbox', { name: 'Tags suggestions' });
  await expect(suggestions).toBeVisible();
  await expect(suggestions.getByRole('option', { name: /blue/ })).toBeVisible();
});

test('Ctrl+Enter commits a pending tag draft and submits tagging', async ({ page }) => {
  const tagRequests = await openSelectedLibrary(page);

  await page.getByRole('button', { name: 'Tag…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  const input = dialog.getByRole('textbox', { name: 'Tags' });
  await input.fill('rating:safe');
  await input.press('Control+Enter');

  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0]).toEqual({ method: 'POST', body: { file_ids: ['one'], tags: ['rating:safe'], verbose: false } });
  await expect(dialog).toHaveCount(0);
});

test('Ctrl+Enter commits a pending tag draft and submits untagging', async ({ page }) => {
  const tagRequests = await openSelectedLibrary(page);

  await page.locator('.sb-actions button').filter({ hasText: 'Untag…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Untag selected files' });
  const input = dialog.getByRole('textbox', { name: 'Tags' });
  await input.fill('blue');
  await input.press('Control+Enter');

  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0]).toEqual({ method: 'DELETE', body: { file_ids: ['one'], tags: ['blue'], verbose: false } });
  await expect(dialog).toHaveCount(0);
});
