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
    can_delete: true,
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
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function selectFirstFile(page: Page) {
  const first = page.getByRole('button', { name: 'Preview one.jpg' });
  await first.focus();
  await page.keyboard.press('Space');
  await expect(page.getByText('1 of 3 selected')).toBeVisible();
}

test('selection toolbar has deliberate labels, icons, and action order', async ({ page }) => {
  await mockApp(page);
  await selectFirstFile(page);

  const selectAll = page.getByRole('button', { name: 'Select all 3' });
  await expect(selectAll).toBeVisible();
  expect((await selectAll.textContent())?.replace(/\u00a0/g, ' ').replace(/\s+/g, ' ').trim()).toBe('Select all 3');

  const actions = page.locator('.sb-actions > button');
  await expect(actions).toHaveCount(6);
  await expect(actions.nth(0)).toHaveAccessibleName('Export');
  await expect(actions.nth(1)).toHaveAccessibleName('Tag…');
  await expect(actions.nth(2)).toHaveAccessibleName('Untag…');
  await expect(actions.nth(3)).toHaveAccessibleName('Untrack');
  await expect(actions.nth(4)).toHaveAccessibleName('Delete');
  await expect(actions.nth(5)).toHaveAccessibleName('Clear selection');

  await expect(actions.nth(2).locator('svg')).toBeVisible();
  await expect(actions.nth(3).locator('svg')).toBeVisible();
});

test('plain Enter confirms no-input removal dialogs even when Cancel owns focus', async ({ page }) => {
  await mockApp(page);
  const removals: unknown[] = [];
  await page.route('**/api/v1/files', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    removals.push(route.request().postDataJSON());
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ mode: 'untrack', removed_locations: 1 }) });
  });

  await selectFirstFile(page);
  await page.keyboard.press('Delete');
  const dialog = page.getByRole('dialog', { name: 'Untrack selected files' });
  await expect(dialog).toBeVisible();

  await dialog.getByRole('button', { name: 'Cancel' }).focus();
  await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
  await page.keyboard.press('Enter');

  await expect.poll(() => removals.length).toBe(1);
  expect(removals[0]).toEqual({ mode: 'untrack', ids: ['one'] });
  await expect(dialog).toHaveCount(0);
});
