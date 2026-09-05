from pathlib import Path

for filename in ['SearchBar.svelte', 'TagAutocompleteInput.svelte']:
    path = Path('frontend/src/lib/components') / filename
    text = path.read_text()
    if "completionVisibility" in text:
        continue
    if filename == 'SearchBar.svelte':
        text = text.replace("<script lang=\"ts\">\n", "<script lang=\"ts\">\n  import { tick } from 'svelte';\n", 1)
        text = text.replace("  import { isEditableShortcutTarget } from '$lib/utils/keyboard';\n", "  import { isEditableShortcutTarget } from '$lib/utils/keyboard';\n  import { keepActiveCompletionVisible } from '$lib/utils/completionVisibility';\n")
        text = text.replace("  let rootRef = $state<HTMLDivElement | undefined>();\n", "  let rootRef = $state<HTMLDivElement | undefined>();\n  let suggestionsRef = $state<HTMLUListElement | undefined>();\n")
        anchor = "  $effect(() => {\n    if (active >= flat.length) active = 0;\n  });\n"
        effect = anchor + "\n  $effect(() => {\n    active;\n    if (!open) return;\n    void tick().then(() => keepActiveCompletionVisible(suggestionsRef));\n  });\n"
        text = text.replace(anchor, effect)
        text = text.replace('<ul id="searchbar-suggestions" class="search-suggestions"', '<ul bind:this={suggestionsRef} id="searchbar-suggestions" class="search-suggestions"')
    else:
        text = text.replace("<script lang=\"ts\">\n", "<script lang=\"ts\">\n  import { tick } from 'svelte';\n", 1)
        text = text.replace("  import { plainTagSuggestions, plainTagsFromInput, type PlainTagSuggestion, type TagCandidate } from '$lib/utils/tagSuggestions';\n", "  import { plainTagSuggestions, plainTagsFromInput, type PlainTagSuggestion, type TagCandidate } from '$lib/utils/tagSuggestions';\n  import { keepActiveCompletionVisible } from '$lib/utils/completionVisibility';\n")
        text = text.replace("  let inputRef = $state<HTMLInputElement | undefined>();\n", "  let inputRef = $state<HTMLInputElement | undefined>();\n  let suggestionsRef = $state<HTMLUListElement | undefined>();\n")
        anchor = "  $effect(() => {\n    if (active >= suggestions.length) active = 0;\n    if (!value.trim() || readOnly) open = false;\n  });\n"
        effect = anchor + "\n  $effect(() => {\n    active;\n    if (!open) return;\n    void tick().then(() => keepActiveCompletionVisible(suggestionsRef));\n  });\n"
        text = text.replace(anchor, effect)
        text = text.replace('<ul class="tag-autocomplete-list"', '<ul bind:this={suggestionsRef} class="tag-autocomplete-list"')
    path.write_text(text)

path = Path('frontend/tests/completion-scroll.spec.ts')
path.write_text(r'''import { expect, test, type Locator, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const manyTags = Array.from({ length: 30 }, (_, index) => ({
  name: `test_tag_${String(index + 1).padStart(2, '0')}`,
  count: 100 - index
}));

function fileItem() {
  return {
    id: 'one', content_id: 'hash-one', name: 'one.jpg', safe_display_path: 'library/one.jpg', size: 2048,
    modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 }, tags: ['blue'],
    media_urls: { thumbnail: '/api/v1/files/one/thumbnail', preview: '/api/v1/files/one/preview', content: '/api/v1/files/one/content', download: '/api/v1/files/one/download' }
  };
}

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: loggedIn ? 200 : 401, contentType: 'application/json', body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } }) }));
  await page.route('**/api/v1/auth/login', async (route) => { loggedIn = true; await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }); });
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: manyTags }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files: [fileItem()], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } }) }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" />' }));
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function expectActiveInside(list: Locator) {
  const result = await list.evaluate((node) => {
    const active = node.querySelector<HTMLElement>('[role="option"][aria-selected="true"]');
    if (!active) return null;
    const box = node.getBoundingClientRect();
    const activeBox = active.getBoundingClientRect();
    return { scrollTop: node.scrollTop, top: activeBox.top, bottom: activeBox.bottom, listTop: box.top, listBottom: box.bottom };
  });
  expect(result).not.toBeNull();
  expect(result!.scrollTop).toBeGreaterThan(0);
  expect(result!.top).toBeGreaterThanOrEqual(result!.listTop - 1);
  expect(result!.bottom).toBeLessThanOrEqual(result!.listBottom + 1);
}

test('main search scrolls keyboard-selected completion into view', async ({ page }) => {
  await mockApp(page);
  const input = page.getByLabel('Search library');
  await input.fill('test');
  const list = page.getByRole('listbox', { name: 'Search suggestions' });
  await expect(list).toBeVisible();
  for (let i = 0; i < 12; i += 1) await input.press('ArrowDown');
  await expectActiveInside(list);
});

test('shared tag completion scrolls keyboard-selected item into view', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('checkbox', { name: 'Select one.jpg' }).click();
  await page.getByRole('button', { name: 'Tag…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  const input = dialog.getByLabel('Tags');
  await input.fill('test');
  const list = dialog.getByRole('listbox', { name: 'Tags suggestions' });
  await expect(list).toBeVisible();
  for (let i = 0; i < 12; i += 1) await input.press('ArrowDown');
  await expectActiveInside(list);
});
''')
