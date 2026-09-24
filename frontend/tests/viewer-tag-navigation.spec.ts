import { mockFileAround } from './helpers/mockFileAround';
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
  const files = [fileItem('one', 'one.jpg'), fileItem('two', 'two.jpg'), fileItem('three', 'three.jpg')];
  let loggedIn = false;
  let tagCreated = false;
  let namespacedTagCreated = false;
  let indexedLookups = 0;

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
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    const query = new URL(route.request().url()).searchParams.get('q');
    if (query === 'tag') indexedLookups += 1;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({
      items: tagCreated && query === 'tag' ? [{ name: 'tag1', count: 1 }] : namespacedTagCreated && query === 'val' ? [{ name: 'test:value', count: 1 }] : []
    }) });
  });
  await page.route('**/api/v1/files/tags', async (route) => {
    const request = route.request().postDataJSON() as { tags: string[] };
    if (request.tags.includes('tag1')) tagCreated = true;
    if (request.tags.includes('test:value')) namespacedTagCreated = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ updated_files: 1 }) });
  });
  await mockFileAround(page, () => files);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32"/></svg>' }));
  await page.route('**/api/v1/files/*/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="64"><rect width="96" height="64"/></svg>' }));
  await page.route('**/api/v1/files/*/content', async (route) => route.fulfill({ status: 404, contentType: 'text/plain', body: 'fixture unavailable' }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return { get tagCreated() { return tagCreated; }, get namespacedTagCreated() { return namespacedTagCreated; }, get indexedLookups() { return indexedLookups; } };
}

test('empty viewer tag field uses Left and Right to leave the editor and navigate', async ({ page }) => {
  await mockApp(page);

  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();

  const twoTags = page.getByLabel('Tags for two.jpg');
  await twoTags.focus();
  await expect(twoTags).toBeFocused();
  await expect(twoTags).toHaveValue('');

  await page.keyboard.press('Shift+ArrowRight');
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
  await expect(twoTags).toBeFocused();

  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'three.jpg' })).toBeVisible();
  const threeTags = page.getByLabel('Tags for three.jpg');
  await expect(threeTags).toBeFocused();

  await threeTags.focus();
  await expect(threeTags).toBeFocused();
  await page.keyboard.press('ArrowLeft');

  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
  await expect(page.getByLabel('Tags for two.jpg')).toBeFocused();
});

test('non-empty viewer tag field keeps Left and Right for caret movement', async ({ page }) => {
  await mockApp(page);

  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'two.jpg' });
  const tagInput = page.getByLabel('Tags for two.jpg');
  await tagInput.fill('portrait');
  await tagInput.evaluate((node) => (node as HTMLInputElement).setSelectionRange(4, 4));

  await page.keyboard.press('ArrowLeft');
  await expect(dialog).toBeVisible();
  await expect.poll(() => tagInput.evaluate((node) => (node as HTMLInputElement).selectionStart)).toBe(3);

  await page.keyboard.press('ArrowRight');
  await expect(dialog).toBeVisible();
  await expect.poll(() => tagInput.evaluate((node) => (node as HTMLInputElement).selectionStart)).toBe(4);
});

test('newly tagged image contributes viewer suggestions on the next image', async ({ page }) => {
  const state = await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const first = page.getByLabel('Tags for one.jpg');
  await first.fill('tag');
  await expect.poll(() => state.indexedLookups).toBeGreaterThan(0);
  await first.fill('tag1');
  await first.press('Space');
  await expect.poll(() => state.tagCreated).toBe(true);
  await expect(first).toHaveValue('');

  await first.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
  await page.getByLabel('Tags for two.jpg').fill('tag');
  await expect(page.getByRole('listbox', { name: 'Tags for two.jpg suggestions' })
    .getByRole('option', { name: /tag1/ })).toBeVisible();
  expect(state.indexedLookups).toBeGreaterThanOrEqual(2);
});

test('new namespaced tags complete by value prefix in viewer and main search', async ({ page }) => {
  const state = await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const first = page.getByLabel('Tags for one.jpg');
  await first.fill('test:value');
  await first.press('Space');
  await expect.poll(() => state.namespacedTagCreated).toBe(true);
  await expect(first).toHaveValue('');
  await first.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'two.jpg' })).toBeVisible();
  const second = page.getByLabel('Tags for two.jpg');
  await second.fill('val');
  await expect(page.getByRole('listbox', { name: 'Tags for two.jpg suggestions' })
    .getByRole('option', { name: /test:value/ })).toBeVisible();
  await page.getByRole('button', { name: 'Close preview' }).click();
  await page.getByLabel('Search library').fill('val');
  await expect(page.getByRole('listbox', { name: 'Search suggestions' })
    .getByRole('option', { name: /test:value/ })).toBeVisible();
});


test('modified viewer arrows are not consumed by either viewer or library fallback', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'two.jpg' });
  await expect(dialog).toBeVisible();
  // Dispatch on the stage and on window: the stage and the library have distinct
  // keydown handlers, and both must leave browser shortcuts alone.
  for (const target of ['stage', 'window'] as const) {
    for (const key of ['ArrowLeft', 'ArrowRight', 'j', 'k']) {
      for (const modifiers of [
        { altKey: true }, { ctrlKey: true }, { metaKey: true },
        { altKey: true, shiftKey: true }, { ctrlKey: true, shiftKey: true },
        { metaKey: true, shiftKey: true }
      ]) {
        const prevented = await page.evaluate(({ target, key, modifiers }) => {
          const owner = target === 'stage' ? document.querySelector('.viewer-stage') : window;
          if (!owner) throw new Error('viewer stage missing');
          const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...modifiers });
          owner.dispatchEvent(event);
          return event.defaultPrevented;
        }, { target, key, modifiers });
        expect(prevented, `${target}: ${key} with ${JSON.stringify(modifiers)}`).toBe(false);
        await expect(dialog).toBeVisible();
      }
    }
  }
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'three.jpg' })).toBeVisible();
});


test('empty viewer tag input switches back to add mode with layout-shifted plus', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview two.jpg' }).click();
  const dialog = page.getByRole('dialog', { name: 'two.jpg' });
  const addInput = dialog.getByRole('textbox', { name: 'Tags for two.jpg' });
  await addInput.focus();

  await addInput.press('-');
  const removeInput = dialog.getByRole('textbox', { name: 'Remove tags from two.jpg' });
  await expect(removeInput).toBeFocused();
  await expect(removeInput).toHaveValue('');

  // On common keyboard layouts the + character is generated by Shift+Equal.
  // It must reach TagEditor's exact Shift++ shortcut, rather than become text.
  await removeInput.press('Shift+Equal');
  await expect(addInput).toBeFocused();
  await expect(addInput).toHaveValue('');

  // Do not broaden the shortcut to include Alt/Ctrl/Meta combinations.
  await addInput.press('-');
  await expect(removeInput).toBeFocused();
  const prevented = await removeInput.evaluate((element) => {
    const event = new KeyboardEvent('keydown', {
      key: '+', code: 'Equal', shiftKey: true, altKey: true,
      bubbles: true, cancelable: true
    });
    element.dispatchEvent(event);
    return event.defaultPrevented;
  });
  expect(prevented).toBe(false);
  await expect(removeInput).toBeFocused();
  await expect(removeInput).toHaveValue('');

  // Mode shortcuts apply only when the tag draft is empty.
  await removeInput.fill('literal');
  await removeInput.press('Shift+Equal');
  await expect(removeInput).toHaveValue('literal+');
});
