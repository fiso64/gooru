import { mockFileAround } from './helpers/mockFileAround';
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
  added_at: '2026-05-20T00:00:00Z',
  modified_time: '2026-05-20T00:00:00Z',
  media_type: 'image/jpeg',
  media_kind: 'photo',
  metadata: { image_width: 800, image_height: 600 },
  tags: [],
  can_delete: true,
  media_urls: {
    thumbnail: '/api/v1/files/one/thumbnail',
    preview: '/api/v1/files/one/preview',
    content: '/api/v1/files/one/content',
    download: '/api/v1/files/one/download'
  }
};

type TagRequest = { method: string; body: { file_ids?: string[]; tags?: string[] } };

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
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ name: 'technology', count: 8 }, { name: 'technique', count: 3 }, { name: 'rating:safe', count: 4 }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [
      { name: 'technology', count: 8 },
      { name: 'technique', count: 3 },
      { name: 'rating:safe', namespace: 'rating', value: 'safe', count: 4 }
    ] })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [{ value: 'photo', count: 1 }] } })
  }));
  await page.route('**/api/v1/files/tags', async (route) => {
    if (!['POST', 'PUT', 'DELETE'].includes(route.request().method())) return route.fallback();
    tagRequests.push({ method: route.request().method(), body: route.request().postDataJSON() });
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ updated_files: 1 }) });
  });
  await page.route('**/api/v1/files/one/thumbnail**', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" />'
  }));
  await page.route('**/api/v1/files/one/preview**', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" />'
  }));

  await mockFileAround(page, () => [file]);
  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return tagRequests;
}

test('viewer tag input dismisses completions before blur, does not commit on blur, and commits on Space', async ({ page }) => {
  const tagRequests = await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();

  const preview = page.getByRole('dialog', { name: 'one.jpg' });
  const aside = preview.locator('.lightbox-aside');
  const input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await input.fill('tech');

  const suggestions = page.getByRole('listbox', { name: 'Tags for one.jpg suggestions' });
  await expect(suggestions).toBeVisible();
  const [asideBox, suggestionsBox] = await Promise.all([aside.boundingBox(), suggestions.boundingBox()]);
  expect(asideBox).not.toBeNull();
  expect(suggestionsBox).not.toBeNull();
  expect(suggestionsBox!.x + suggestionsBox!.width).toBeLessThanOrEqual(asideBox!.x + asideBox!.width);

  await input.press('Escape');
  await expect(suggestions).toBeHidden();
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('tech');
  expect(tagRequests).toHaveLength(0);

  await preview.getByRole('button', { name: 'Close preview' }).focus();
  await expect(input).not.toBeFocused();
  expect(tagRequests).toHaveLength(0);

  await input.focus();
  await input.fill('tech');
  await expect(suggestions).toBeVisible();
  await input.press('Space');
  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0]).toMatchObject({ method: 'POST', body: { file_ids: ['one'], tags: ['tech'] } });
});


test('confirmed viewer tag appears while tag-index refresh is pending', async ({ page }) => {
  const requests = await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  let finish: () => void = () => {};
  const gate = new Promise<void>((resolve) => { finish = () => resolve(); });
  let refreshing = false;
  await page.route('**/api/v1/tags?**', async (route) => {
    if (requests.length) {
      refreshing = true;
      await gate;
    }
    await route.fallback();
  });
  try {
    const input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
    await input.fill('fresh-tag');
    await input.press('Enter');
    await expect.poll(() => requests.length).toBe(1);
    await expect.poll(() => refreshing).toBe(true);
    await expect(page.getByRole('button', { name: 'Search for fresh-tag' })).toBeVisible();
  } finally {
    finish();
  }
});

test('Tab inserts a highlighted viewer suggestion without committing it; Enter commits the draft', async ({ page }) => {
  const tagRequests = await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();

  const input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await input.fill('tech');
  const suggestions = page.getByRole('listbox', { name: 'Tags for one.jpg suggestions' });
  await expect(suggestions.getByRole('option').first()).toContainText('technology');

  await input.press('Tab');
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('technology');
  await expect(suggestions).toBeHidden();
  expect(tagRequests).toHaveLength(0);

  await input.press('Enter');
  await expect.poll(() => tagRequests.length).toBe(1);
  expect(tagRequests[0]).toMatchObject({ method: 'POST', body: { file_ids: ['one'], tags: ['technology'] } });
});

test('Tab completes a bulk-tag suggestion in the editor, without staging or submitting it', async ({ page }) => {
  const tagRequests = await mockApp(page);
  await page.getByRole('checkbox', { name: 'Select one.jpg' }).click();
  await page.getByRole('button', { name: 'Tag…' }).click();

  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  const input = dialog.getByRole('textbox', { name: 'Tags' });
  await input.fill('tech');
  const suggestions = dialog.getByRole('listbox', { name: 'Tags suggestions' });
  await expect(suggestions.getByRole('option').first()).toContainText('technology');

  await input.press('Tab');
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('technology');
  await expect(suggestions).toBeHidden();
  expect(tagRequests).toHaveLength(0);

  await input.press('Enter');
  await expect(dialog.getByText('technology', { exact: true })).toBeVisible();
  expect(tagRequests).toHaveLength(0);
});

test('tag modal keeps open on first Escape and Space commits the literal prefix', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('checkbox', { name: 'Select one.jpg' }).click();
  await page.getByRole('button', { name: 'Tag…' }).click();

  const dialog = page.getByRole('dialog', { name: 'Tag selected files' });
  const input = dialog.getByRole('textbox', { name: 'Tags' });
  await input.fill('tech');
  const suggestions = dialog.getByRole('listbox', { name: 'Tags suggestions' });
  await expect(suggestions).toBeVisible();

  await input.press('Escape');
  await expect(suggestions).toBeHidden();
  await expect(dialog).toBeVisible();
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('tech');

  await input.press('Space');
  await expect(dialog.getByText('tech', { exact: true })).toBeVisible();
  await expect(input).toHaveValue('');

  await input.press('Escape');
  await expect(dialog).toHaveCount(0);
});

test('viewer +/- mode control supports mouse toggling while punctuation remains tag text', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();

  await page.getByRole('button', { name: 'Switch to removing tags from one.jpg' }).click();
  let input = page.getByRole('textbox', { name: 'Remove tags from one.jpg' });
  await expect(input).toBeFocused();
  await page.getByRole('button', { name: 'Switch to adding tags for one.jpg' }).click();
  input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await expect(input).toBeFocused();

  await input.press('-');
  input = page.getByRole('textbox', { name: 'Remove tags from one.jpg' });
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('');
  await input.press('+');
  input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await expect(input).toBeFocused();
  await expect(input).toHaveValue('');

  await input.fill('rating:safe');
  await input.press('-');
  await expect(input).toHaveValue('rating:safe-');
  await expect(page.getByRole('textbox', { name: 'Tags for one.jpg' })).toBeFocused();
});


test('tag completion remains mounted while the next matching query is pending', async ({ page }) => {
  await mockApp(page);
  await page.unroute('**/api/v1/search/suggestions?**');
  let release!: () => void;
  const gate = new Promise<void>((resolve) => { release = resolve; });
  let pending = false;
  await page.route('**/api/v1/search/suggestions?**', async (route) => {
    const query = new URL(route.request().url()).searchParams.get('q');
    if (query === 'techn') { pending = true; await gate; }
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ name: 'technology', count: 8 }] }) });
  });
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  const list = page.getByRole('listbox', { name: 'Tags for one.jpg suggestions' });
  await input.fill('tech');
  await expect(list.getByRole('option').first()).toContainText('technology');
  await list.evaluate((node) => { (node as HTMLElement).dataset.popupIdentity = 'first'; });
  await input.press('n');
  await expect.poll(() => pending).toBe(true);
  await expect(list).toHaveAttribute('data-popup-identity', 'first');
  await expect(list.getByRole('option').first()).toContainText('technology');
  release();
});


test('viewer tag completions ellipsize long names without horizontal scrolling', async ({ page }) => {
  await mockApp(page);
  await page.unroute('**/api/v1/search/suggestions?**');
  const longTag = `character:${'very_long_unbroken_name_'.repeat(12)}`;
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ name: longTag, count: 123456 }] })
  }));
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await input.locator('..').evaluate((node) => {
    const element = node as HTMLElement;
    element.style.width = '170px';
    element.style.maxWidth = '170px';
    element.style.flex = '0 0 170px';
  });
  await input.fill('character:v');
  const list = page.getByRole('listbox', { name: 'Tags for one.jpg suggestions' });
  await expect(list.getByRole('option').first()).toBeVisible();
  await expect(list.locator('.name').first()).toHaveAttribute('title', longTag);
  const result = await list.evaluate((node) => {
    const label = node.querySelector<HTMLElement>('.name')!;
    return {
      listWidth: node.clientWidth,
      scrollWidth: node.scrollWidth,
      labelWidth: label.clientWidth,
      labelScrollWidth: label.scrollWidth,
      textOverflow: getComputedStyle(label).textOverflow,
      countWidth: node.querySelector<HTMLElement>('.count')?.getBoundingClientRect().width ?? 0
    };
  });
  expect(result.scrollWidth).toBeLessThanOrEqual(result.listWidth);
  expect(result.labelScrollWidth).toBeGreaterThan(result.labelWidth);
  expect(result.textOverflow).toBe('ellipsis');
  expect(result.countWidth).toBeGreaterThan(0);
});


test('search and viewer tag suggestion menus wrap at both ends', async ({ page }) => {
  await mockApp(page);

  const search = page.getByRole('textbox', { name: 'Search library' });
  await search.fill('tech');
  const searchOptions = page.getByRole('listbox', { name: 'Search suggestions' }).getByRole('option');
  await expect(searchOptions.last()).toBeVisible();
  await expect(searchOptions.first()).toHaveAttribute('aria-selected', 'true');
  await search.press('ArrowUp');
  await expect(searchOptions.last()).toHaveAttribute('aria-selected', 'true');
  await search.press('ArrowDown');
  await expect(searchOptions.first()).toHaveAttribute('aria-selected', 'true');

  await search.fill('');
  await page.getByRole('button', { name: 'Preview one.jpg' }).click();
  const input = page.getByRole('textbox', { name: 'Tags for one.jpg' });
  await input.fill('tech');
  const tagOptions = page.getByRole('listbox', { name: 'Tags for one.jpg suggestions' }).getByRole('option');
  await expect(tagOptions.last()).toBeVisible();
  await expect(tagOptions.first()).toHaveAttribute('aria-selected', 'true');
  await input.press('ArrowUp');
  await expect(tagOptions.last()).toHaveAttribute('aria-selected', 'true');
  await input.press('ArrowDown');
  await expect(tagOptions.first()).toHaveAttribute('aria-selected', 'true');
});
