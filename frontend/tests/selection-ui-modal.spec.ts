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
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/file-selections', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'selection-one', count: files.length }) });
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
  await page.keyboard.press('a');
  await expect(page.getByText('3 selected')).toBeVisible();
  await page.getByRole('checkbox', { name: 'Deselect one.jpg' }).click();
  await expect(page.getByText('2 selected')).toBeVisible();
}

test('select all is immediate while snapshot count hydrates independently', async ({ page }) => {
  await mockApp(page);
  let releaseSnapshot!: () => void;
  const snapshotGate = new Promise<void>((resolve) => { releaseSnapshot = resolve; });

  await page.route('**/api/v1/file-selections', async (route) => {
    if (route.request().method() !== 'POST') return route.fallback();
    await snapshotGate;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'selection-delayed', count: 5 }) });
  });

  await page.keyboard.press('a');
  await expect(page.getByText('3 selected')).toBeVisible();
  await expect(page.locator('.thumb.is-selected')).toHaveCount(3);

  releaseSnapshot();
  await expect(page.getByText('5 selected')).toBeVisible();
});

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
  expect(removals[0]).toEqual({ mode: 'untrack', selection_id: 'selection-one', exclude_file_ids: ['one'] });
  await expect(dialog).toHaveCount(0);

  await selectFirstFile(page);
  await page.keyboard.press('Shift+Delete');
  dialog = page.getByRole('dialog', { name: 'Delete selected files' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Cancel' }).focus();
  await page.keyboard.press('Enter');

  await expect.poll(() => removals.length).toBe(2);
  expect(removals[1]).toEqual({ mode: 'delete', selection_id: 'selection-one', exclude_file_ids: ['one'] });
  await expect(dialog).toHaveCount(0);
});

test('bulk removal refreshes the grid only after durable removal completes', async ({ page }) => {
  await mockApp(page);
  const allFiles = [fileItem('one', 'one.jpg'), fileItem('two', 'two.jpg'), fileItem('three', 'three.jpg')];
  let completed = false;
  let releaseOperation!: () => void;
  const operationGate = new Promise<void>((resolve) => { releaseOperation = resolve; });

  await page.route('**/api/v1/files?**', async (route) => {
    const files = completed ? [] : allFiles;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
    });
  });
  await page.route('**/api/v1/files', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({ mode: 'untrack', operation_id: 'remove-one', removed_locations: 0 })
    });
  });
  await page.route('**/api/v1/operations/remove-one', async (route) => {
    await operationGate;
    completed = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'remove-one', status: 'completed' }) });
  });

  await page.keyboard.press('a');
  await page.keyboard.press('Delete');
  const dialog = page.getByRole('dialog', { name: 'Untrack selected files' });
  await dialog.getByRole('button', { name: 'Untrack' }).click();

  await expect(dialog).toBeVisible();
  await expect(page.locator('.thumb')).toHaveCount(3);
  releaseOperation();
  await expect(dialog).toHaveCount(0);
  await expect(page.locator('.thumb')).toHaveCount(0);
});


test('mixed delete asks to untrack external files before one confirmed removal request', async ({ page }) => {
  await mockApp(page);
  const removals: Array<Record<string, unknown>> = [];
  await page.route('**/api/v1/files', async (route) => {
    if (route.request().method() !== 'DELETE') return route.fallback();
    const removal = route.request().postDataJSON() as Record<string, unknown>;
    removals.push(removal);
    if (removal.mode === 'delete') {
      await route.fulfill({
        status: 409,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'file_not_managed', message: 'one or more selected files are outside configured upload targets; untrack them instead' } })
      });
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ mode: removal.mode, removed_locations: 2, selector: {} })
    });
  });

  await selectFirstFile(page);
  await page.keyboard.press('Shift+Delete');
  let dialog = page.getByRole('dialog', { name: 'Delete selected files' });
  await dialog.getByRole('button', { name: 'Delete files' }).click();

  dialog = page.getByRole('dialog', { name: 'Delete managed files and untrack external files?' });
  await expect(dialog).toBeVisible();
  await expect(dialog).toContainText('External files will remain on disk.');
  expect(removals).toEqual([{ mode: 'delete', selection_id: 'selection-one', exclude_file_ids: ['one'] }]);

  await dialog.getByRole('button', { name: 'Delete and untrack' }).click();
  await expect.poll(() => removals.length).toBe(2);
  expect(removals[1]).toEqual({ mode: 'delete_or_untrack', selection_id: 'selection-one', exclude_file_ids: ['one'] });
  await expect(dialog).toHaveCount(0);
});
