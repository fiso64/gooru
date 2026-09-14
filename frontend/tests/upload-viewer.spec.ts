import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'viewer', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-viewer'
};

const png = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z1ioAAAAASUVORK5CYII=',
  'base64'
);
const pngDataURL = `data:image/png;base64,${png.toString('base64')}`;

type TagMutation = { method: string; body: { file_ids?: string[]; tags?: string[]; verbose?: boolean } };
type FileRemoval = { method: string; body: { mode?: string } };

async function mockUploadApp(page: Page, options: {
  completeAsDuplicate?: boolean;
  tagMutations?: TagMutation[];
  tagMutationGate?: Promise<void>;
  fileRemovals?: FileRemoval[];
} = {}) {
  let loggedIn = false;
  let uploadIndex = 0;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      tags: [
        { name: 'artist:alice', count: 8 },
        { name: 'artist:alina', count: 5 },
        { name: 'artist:alex', count: 3 },
        { name: 'artist:amelia', count: 2 }
      ]
    })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files/file-existing', async (route) => {
    const request = route.request();
    if (request.method() === 'DELETE') {
      options.fileRemovals?.push({ method: request.method(), body: JSON.parse(request.postData() || '{}') as FileRemoval['body'] });
      await route.fulfill({ status: 204 });
      return;
    }
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        id: 'file-existing',
        content_id: 'content-existing',
        name: 'duplicate.png',
        safe_display_path: 'duplicate.png',
        size: png.length,
        added_at: '2026-09-14T00:00:00Z',
        modified_time: '2026-09-14T00:00:00Z',
        media_type: 'image/png',
        media_kind: 'photo',
        viewer_support: 'supported',
        can_delete: true,
        metadata: {},
        tags: ['submitted', 'remote:existing'],
        media_urls: { thumbnail: pngDataURL, preview: pngDataURL, content: pngDataURL, download: pngDataURL }
      })
    });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    const request = route.request();
    const body = JSON.parse(request.postData() || '{}') as TagMutation['body'];
    options.tagMutations?.push({ method: request.method(), body });
    if (options.tagMutationGate) await options.tagMutationGate;
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ operation: request.method() === 'DELETE' ? 'remove' : request.method() === 'PUT' ? 'set' : 'add', selector: { file_ids: body.file_ids ?? [] }, affected_count: 1 })
    });
  });
  await page.route('**/api/v1/uploads', async (route) => {
    uploadIndex += 1;
    await route.fulfill({
      status: 202,
      contentType: 'application/json',
      body: JSON.stringify({
        id: `job-${uploadIndex}`,
        kind: 'upload_import',
        status: 'pending',
        progress_total: 1,
        progress_completed: 0,
        progress_failed: 0,
        created_at: '2026-09-14T00:00:00Z'
      })
    });
  });
  await page.route('**/api/v1/operations?**', async (route) => {
    const url = new URL(route.request().url());
    const ids = url.searchParams.getAll('id');
    const items = ids.map((id) => options.completeAsDuplicate ? {
      id,
      kind: 'upload_import',
      status: 'completed',
      progress: 1,
      progress_total: 1,
      progress_completed: 1,
      progress_completed_prefix: 1,
      progress_failed: 0,
      affected_count: 0,
      created_at: '2026-09-14T00:00:00Z',
      finished_at: '2026-09-14T00:00:01Z',
      result: {
        affected_count: 0,
        files: [{ id: 'file-existing', name: 'duplicate.png', size: png.length, target_id: 'default', status: 'duplicate_existing' }]
      }
    } : {
      id,
      kind: 'upload_import',
      status: 'pending',
      progress_total: 1,
      progress_completed: 0,
      progress_failed: 0,
      created_at: '2026-09-14T00:00:00Z'
    });
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items, active_count: options.completeAsDuplicate ? 0 : items.length, total_count: items.length })
    });
  });

  await page.goto('/');
  await page.getByLabel('Username').fill('viewer');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload' }).click();
}

test('staged rows open as a whole, use viewer shortcuts, and edit the underlying tags', async ({ page }) => {
  await mockUploadApp(page);
  await page.locator('input[type="file"]').setInputFiles([
    { name: 'alpha.png', mimeType: 'image/png', buffer: png },
    { name: 'beta.png', mimeType: 'image/png', buffer: png },
    { name: 'book.cbz', mimeType: 'application/vnd.comicbook+zip', buffer: Buffer.from('not-a-real-archive') }
  ]);

  await page.getByLabel('Filter staged files').fill('alpha');
  const alphaRow = page.getByTestId('upload-row-0');
  await expect(alphaRow).toBeVisible();
  await alphaRow.hover();
  await expect.poll(() => alphaRow.evaluate((element) => getComputedStyle(element).cursor)).toBe('pointer');
  await expect(alphaRow.getByRole('button', { name: 'alpha.png', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Preview beta.png' })).toHaveCount(0);

  // The filename is plain text; clicking its row surface opens the viewer.
  await alphaRow.getByRole('button', { name: 'Preview alpha.png' }).click();
  const alphaDialog = page.getByRole('dialog', { name: 'alpha.png' });
  await expect(alphaDialog).toBeVisible();
  await expect(alphaDialog.getByRole('button', { name: 'Toggle fullscreen' })).toBeVisible();
  await expect(alphaDialog.getByRole('button', { name: 'Remove alpha.png from staging' })).toBeVisible();

  const alphaTagInput = page.getByLabel('Tags for alpha.png');
  await alphaTagInput.press('-');
  await expect(page.getByLabel('Remove tags from alpha.png')).toBeFocused();
  await page.getByLabel('Remove tags from alpha.png').press('+');
  await expect(page.getByLabel('Tags for alpha.png')).toBeFocused();

  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'beta.png' })).toBeVisible();
  await page.keyboard.press('ArrowLeft');
  await expect(page.getByRole('dialog', { name: 'alpha.png' })).toBeVisible();
  await page.keyboard.press('ArrowRight');
  const dialog = page.getByRole('dialog', { name: 'beta.png' });
  await expect(dialog).toBeVisible();

  const viewerTagInput = page.getByLabel('Tags for beta.png');
  await viewerTagInput.fill('viewer:edited');
  await viewerTagInput.press('Enter');
  await page.getByRole('button', { name: 'Close upload preview' }).click();

  await page.getByLabel('Filter staged files').fill('');
  await expect(page.getByRole('button', { name: 'Remove viewer:edited from beta.png' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Preview book.cbz' })).toBeDisabled();

  await page.getByTestId('upload-row-0').getByRole('button', { name: 'Preview alpha.png' }).click();
  await page.keyboard.press('Delete');
  await expect(page.getByRole('dialog', { name: 'alpha.png' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Preview alpha.png' })).toHaveCount(0);
});

test('staged tag occurrences augment backend completion counts', async ({ page }) => {
  await mockUploadApp(page);
  const initialTags = page.getByLabel('Initial tags');
  await initialTags.fill('artist:alice local:staged');
  await initialTags.press('Enter');
  await page.locator('input[type="file"]').setInputFiles({ name: 'tagged.png', mimeType: 'image/png', buffer: png });

  await page.getByRole('button', { name: 'Remove artist:alice', exact: true }).click();
  await page.getByRole('button', { name: 'Remove local:staged', exact: true }).click();
  await page.locator('input[type="file"]').setInputFiles({ name: 'plain.png', mimeType: 'image/png', buffer: png });

  const plainInput = page.getByRole('textbox', { name: 'Add tag to plain.png' });
  await plainInput.fill('artist:ali');
  let suggestions = page.getByRole('listbox', { name: 'Add tag to plain.png suggestions' });
  const backendPlusStaged = suggestions.getByRole('option').filter({ hasText: 'artist:alice' });
  await expect(backendPlusStaged).toContainText('9');

  await plainInput.fill('local:sta');
  suggestions = page.getByRole('listbox', { name: 'Add tag to plain.png suggestions' });
  const stagedOnly = suggestions.getByRole('option').filter({ hasText: 'local:staged' });
  await expect(stagedOnly).toContainText('1');
});

test('inline tag completions overlay the last batch row without resizing the card', async ({ page }) => {
  await mockUploadApp(page);
  await page.locator('input[type="file"]').setInputFiles({ name: 'last.png', mimeType: 'image/png', buffer: png });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();

  const batch = page.getByTestId('upload-queue-batch');
  const tagInput = page.getByRole('textbox', { name: 'Add tag to last.png' });
  await expect(batch).toBeVisible();
  await expect(tagInput).toBeVisible();
  const before = await batch.boundingBox();
  if (!before) throw new Error('batch card has no layout box');

  await tagInput.fill('artist:a');
  const suggestions = page.getByRole('listbox', { name: 'Add tag to last.png suggestions' });
  await expect(suggestions).toBeVisible();
  await expect.poll(() => batch.evaluate((element) => getComputedStyle(element).overflow)).toBe('visible');

  const after = await batch.boundingBox();
  const inputBox = await tagInput.boundingBox();
  const suggestionsBox = await suggestions.boundingBox();
  if (!after || !inputBox || !suggestionsBox) throw new Error('tag overlay has no layout box');
  expect(Math.abs(after.height - before.height)).toBeLessThan(1);
  expect(suggestionsBox.y + suggestionsBox.height).toBeLessThan(inputBox.y);
});

test('queue viewer navigates across upload batches as one set', async ({ page }) => {
  await mockUploadApp(page);
  const chooser = page.locator('input[type="file"]');

  await chooser.setInputFiles({ name: 'first.png', mimeType: 'image/png', buffer: png });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();
  await expect(page.getByRole('button', { name: 'Preview first.png' })).toBeVisible();

  await chooser.setInputFiles({ name: 'second.png', mimeType: 'image/png', buffer: png });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();
  await expect(page.getByTestId('upload-queue-batch')).toHaveCount(2);
  await expect(page.getByRole('button', { name: 'Preview second.png' })).toBeVisible();

  await page.getByRole('button', { name: 'Preview first.png' }).click();
  const dialog = page.getByRole('dialog', { name: 'first.png' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Next file' }).click();
  await expect(page.getByRole('dialog', { name: 'second.png' })).toBeVisible();
});

test('uploaded viewer uses the normal toolbar and removal shortcut', async ({ page }) => {
  const fileRemovals: FileRemoval[] = [];
  await mockUploadApp(page, { completeAsDuplicate: true, fileRemovals });

  await page.locator('input[type="file"]').setInputFiles({ name: 'duplicate.png', mimeType: 'image/png', buffer: png });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();
  await expect(page.getByText('duplicate existing', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Preview duplicate.png' }).click();

  const dialog = page.getByRole('dialog', { name: 'duplicate.png' });
  await expect(dialog.getByRole('button', { name: 'Toggle fullscreen' })).toBeVisible();
  await expect(dialog.getByRole('link', { name: 'Download duplicate.png' })).toBeVisible();
  await expect(dialog.getByRole('link', { name: 'Open original duplicate.png' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Delete duplicate.png from disk' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Untrack duplicate.png from library' })).toBeVisible();

  await page.keyboard.press('Delete');
  const removalDialog = page.getByRole('dialog', { name: 'Remove from library' });
  await expect(removalDialog).toBeVisible();
  await expect(dialog).toBeVisible();
  await removalDialog.getByRole('button', { name: 'Remove' }).click();
  await expect.poll(() => fileRemovals).toEqual([{ method: 'DELETE', body: { mode: 'untrack' } }]);
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Preview duplicate.png' })).toHaveCount(0);
});

test('duplicate viewer removes pre-existing remote tags with a delta mutation', async ({ page }) => {
  const tagMutations: TagMutation[] = [];
  await mockUploadApp(page, { completeAsDuplicate: true, tagMutations });

  const initialTags = page.getByLabel('Initial tags');
  await initialTags.fill('submitted');
  await initialTags.press('Enter');
  await page.locator('input[type="file"]').setInputFiles({ name: 'duplicate.png', mimeType: 'image/png', buffer: png });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();

  await expect(page.getByText('duplicate existing', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Preview duplicate.png' }).click();
  const dialog = page.getByRole('dialog', { name: 'duplicate.png' });
  await expect(dialog).toBeVisible();
  const remoteTagRemove = dialog.getByRole('button', { name: 'Remove remote:existing' });
  await expect(remoteTagRemove).toBeVisible();

  await remoteTagRemove.click();
  await expect.poll(() => tagMutations).toEqual([
    { method: 'DELETE', body: { file_ids: ['file-existing'], tags: ['remote:existing'], verbose: false } }
  ]);
  await expect(remoteTagRemove).toHaveCount(0);
  await expect(page.getByTestId('upload-queue-batch').getByRole('button', { name: 'Remove remote:existing from duplicate.png' })).toHaveCount(0);
});

test('completed row stays stable while a direct tag save settles', async ({ page }) => {
  let releaseTagMutation: (() => void) | undefined;
  const tagMutationGate = new Promise<void>((resolve) => { releaseTagMutation = resolve; });
  await mockUploadApp(page, { completeAsDuplicate: true, tagMutationGate });

  await page.locator('input[type="file"]').setInputFiles({ name: 'duplicate.png', mimeType: 'image/png', buffer: png });
  await page.getByRole('button', { name: 'Upload 1 file' }).click();
  await expect(page.getByText('duplicate existing', { exact: true })).toBeVisible();
  const row = page.getByTestId('upload-row-0');
  const before = await row.boundingBox();
  if (!before) throw new Error('completed row has no layout box');

  await page.getByRole('button', { name: 'Preview duplicate.png' }).click();
  const dialog = page.getByRole('dialog', { name: 'duplicate.png' });
  const remoteTagRemove = dialog.getByRole('button', { name: 'Remove remote:existing' });
  await expect(remoteTagRemove).toBeVisible();
  await remoteTagRemove.click();
  await page.getByRole('button', { name: 'Close preview' }).click();

  await expect(row.getByText('Saving tag changes…')).toHaveCount(0);
  const after = await row.boundingBox();
  if (!after) throw new Error('completed row disappeared during tag save');
  expect(Math.abs(after.height - before.height)).toBeLessThan(1);

  await page.getByRole('button', { name: 'Clear done' }).click();
  await expect(page.getByRole('button', { name: 'Preview duplicate.png' })).toBeVisible();

  releaseTagMutation?.();
  await expect.poll(async () => page.getByRole('button', { name: 'Clear done' }).isEnabled()).toBe(true);
  await page.getByRole('button', { name: 'Clear done' }).click();
  await expect(page.getByRole('button', { name: 'Preview duplicate.png' })).toHaveCount(0);
});
