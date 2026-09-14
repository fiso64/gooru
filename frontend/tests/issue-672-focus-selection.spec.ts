import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_672', username: 'issue-672', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-672'
};

const file = {
  id: 'issue-672-image',
  content_id: 'hash-issue-672-image',
  name: 'focus-test.jpg',
  safe_display_path: 'library/focus-test.jpg',
  size: 2048,
  added_at: '2026-09-14T00:00:00Z',
  modified_time: '2026-09-14T00:00:00Z',
  media_type: 'image/jpeg',
  media_kind: 'photo',
  viewer_support: 'supported',
  metadata: { image_width: 4000, image_height: 3000 },
  tags: [],
  can_delete: true,
  media_urls: {
    thumbnail: '/api/v1/files/issue-672-image/thumbnail',
    preview: '/api/v1/files/issue-672-image/preview',
    content: '/api/v1/files/issue-672-image/content',
    download: '/api/v1/files/issue-672-image/download'
  }
};

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/jobs?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [], facets: { kind: [] } }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [] } })
  }));
  const thumbnail = '<svg xmlns="http://www.w3.org/2000/svg" width="64" height="48"><rect width="64" height="48"/></svg>';
  const preview = '<svg xmlns="http://www.w3.org/2000/svg" width="4000" height="3000"><rect width="4000" height="3000"/></svg>';
  await page.route('**/api/v1/files/issue-672-image/thumbnail**', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: thumbnail }));
  await page.route('**/api/v1/files/issue-672-image/preview**', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: preview }));
  await page.route('**/api/v1/files/issue-672-image/content**', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: preview }));

  await page.goto('/');
  await page.getByLabel('Username').fill('issue-672');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('pointer-opened grid item does not gain a keyboard cursor after closing the viewer', async ({ page }) => {
  await mockApp(page);
  const card = page.getByRole('button', { name: 'Preview focus-test.jpg' });

  await card.click();
  await expect(page.getByRole('dialog', { name: 'focus-test.jpg' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'focus-test.jpg' })).toBeHidden();

  await expect(card).not.toBeFocused();
  await expect.poll(async () => card.evaluate((node) => getComputedStyle(node, '::after').content)).toBe('none');
});

test('keyboard-opened grid item still restores its visible keyboard cursor', async ({ page }) => {
  await mockApp(page);
  const card = page.getByRole('button', { name: 'Preview focus-test.jpg' });

  await page.keyboard.press('ArrowDown');
  await expect(card).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('dialog', { name: 'focus-test.jpg' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'focus-test.jpg' })).toBeHidden();

  await expect(card).toBeFocused();
  const cursor = await card.evaluate((node) => {
    const style = getComputedStyle(node, '::after');
    return { content: style.content, borderStyle: style.borderStyle };
  });
  expect(cursor.content).not.toBe('none');
  expect(cursor.borderStyle).toBe('dashed');
});

test('viewer media focus has no stage ring and media cannot be browser-selected', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview focus-test.jpg' }).click();

  const dialog = page.getByRole('dialog', { name: 'focus-test.jpg' });
  const stage = dialog.locator('.viewer-stage');
  const image = stage.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();

  await stage.focus();
  await expect(stage).toBeFocused();
  const stageFocus = await stage.evaluate((node) => {
    const style = getComputedStyle(node);
    return { outlineStyle: style.outlineStyle, outlineWidth: style.outlineWidth };
  });
  expect(stageFocus.outlineStyle).toBe('none');
  expect(stageFocus.outlineWidth).toBe('0px');
  await expect.poll(async () => image.evaluate((node) => getComputedStyle(node).userSelect)).toBe('none');
});
