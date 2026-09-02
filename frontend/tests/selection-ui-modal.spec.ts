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
  await page.getByLabel('Select all files in current view').click();
  await expect(page.getByText('3 of 3 selected')).toBeVisible();
  await page.locator('.thumb-open[aria-label="Deselect one.jpg"]').click();
  await expect(page.getByText('2 of 3 selected')).toBeVisible();
}

test('selection toolbar exposes Select all with a real visible gap', async ({ page }) => {
  await mockApp(page);
  await selectFirstFile(page);
  const selectAll = page.locator('.selection-summary button');
  await expect(selectAll).toBeVisible();
  const parts = selectAll.locator(':scope > span');
  await expect(parts).toHaveCount(2);
  const [selectBox, allBox] = await Promise.all([parts.nth(0).boundingBox(), parts.nth(1).boundingBox()]);
  expect(selectBox).not.toBeNull();
  expect(allBox).not.toBeNull();
  expect(allBox!.x - (selectBox!.x + selectBox!.width)).toBeGreaterThan(1);
});

test('selection toolbar action order and labels match the owner request', async ({ page }) => {
  await mockApp(page);
  await selectFirstFile(page);
  const actions = page.locator('.sb-actions > button');
  await expect(actions).toHaveCount(6);
  await expect(actions.nth(0)).toHaveAccessibleName('Export');
  await expect(actions.nth(1)).toHaveAccessibleName(/T\s+ag…/);
  await expect(actions.nth(2)).toHaveAccessibleName(/U\s+ntag…/);
  await expect(actions.nth(3)).toHaveAccessibleName('Untrack');
  await expect(actions.nth(4)).toHaveAccessibleName('Delete');
  await expect(actions.nth(5)).toHaveAccessibleName('Clear selection');
});

test('selection toolbar gives Untag and Untrack distinct icons', async ({ page }) => {
  await mockApp(page);
  await selectFirstFile(page);
  const actions = page.locator('.sb-actions > button');
  await expect(actions.nth(2).locator('svg')).toBeVisible();
  await expect(actions.nth(3).locator('svg')).toBeVisible();
  const untagPath = await actions.nth(2).locator('svg').innerHTML();
  const untrackPath = await actions.nth(3).locator('svg').innerHTML();
  const deletePath = await actions.nth(4).locator('svg').innerHTML();
  expect(untagPath).not.toBe(deletePath);
  expect(untrackPath).not.toBe(deletePath);
  expect(untrackPath).not.toBe(untagPath);
  expect(untrackPath).toContain('m19.5 10 2 2-2 2');
});

test('plain Enter confirms no-input Untrack and Delete dialogs even when Cancel owns focus', async ({ page }) => {
  await mockApp(page);
  const removals: unknown[] = [];
  await page.route('**/api/v1/files', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    const removal = route.request().postDataJSON();
    removals.push(removal);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ mode: (removal as { mode: string }).mode, removed_locations: 1 })
    });
  });

  await selectFirstFile(page);
  await page.keyboard.press('Delete');
  let dialog = page.getByRole('dialog', { name: 'Untrack selected files' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).focus();
  await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
  await page.keyboard.press('Enter');

  await expect.poll(() => removals.length).toBe(1);
  expect(removals[0]).toEqual({ mode: 'untrack', query: '*', exclude_file_ids: ['one'] });
  await expect(dialog).toHaveCount(0);

  await selectFirstFile(page);
  await page.keyboard.press('Shift+Delete');
  dialog = page.getByRole('dialog', { name: 'Delete selected files' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).focus();
  await page.keyboard.press('Enter');

  await expect.poll(() => removals.length).toBe(2);
  expect(removals[1]).toEqual({ mode: 'delete', query: '*', exclude_file_ids: ['one'] });
  await expect(dialog).toHaveCount(0);
});
