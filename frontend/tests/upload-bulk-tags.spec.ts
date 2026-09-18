import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'tester', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-test'
};

async function mockApp(page: Page) {
  let loggedIn = false;
  await page.route('**/api/v1/auth/me', (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', (route) => {
    loggedIn = true;
    return route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/files?**', (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/operations**', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ items: [{ id: 'default', name: 'Default inbox' }] })
  }));
  await page.route('**/api/v1/tags?**', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
}

async function openUpload(page: Page) {
  await page.goto('/');
  await page.getByLabel('Username').fill('tester');
  await page.getByLabel('Password').fill('test');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await page.getByRole('button', { name: 'Upload', exact: true }).click();
}

test('bulk staged tag controls add, remove, set, clear, and explain initial-tag drift', async ({ page }) => {
  await mockApp(page);
  await openUpload(page);

  const initial = page.getByLabel('Initial tags');
  await initial.fill('initial');
  await initial.press('Enter');

  await page.locator('input[type="file"]').setInputFiles([
    { name: 'first.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('first') },
    { name: 'second.jpg', mimeType: 'image/jpeg', buffer: Buffer.from('second') }
  ]);

  const addTags = page.getByRole('button', { name: 'Add tags to staged files' });
  const removeTags = page.getByRole('button', { name: 'Remove tags from staged files' });
  await expect(addTags).toHaveText('');
  await expect(removeTags).toHaveText('');
  await expect(addTags.locator('svg')).toHaveCount(1);
  await expect(removeTags.locator('svg')).toHaveCount(1);
  const [addBox, removeBox] = await Promise.all([addTags.boundingBox(), removeTags.boundingBox()]);
  expect(addBox).not.toBeNull();
  expect(removeBox).not.toBeNull();
  expect(addBox!.width).toBe(addBox!.height);
  expect(removeBox!.width).toBe(removeBox!.height);

  await initial.fill('future');
  await initial.press('Enter');
  await expect(page.getByText(/Initial tag changes affect newly staged files only/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Remove future from first.jpg' })).toHaveCount(0);

  await page.getByRole('button', { name: 'SET', exact: true }).click();
  await expect(page.getByText(/Initial tag changes affect newly staged files only/)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Remove future from first.jpg' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Remove future from second.jpg' })).toBeVisible();

  await page.getByRole('button', { name: 'Add tags to staged files' }).click();
  const addDialog = page.getByRole('dialog');
  await addDialog.getByRole('textbox', { name: 'Tags' }).fill('bulk');
  await addDialog.getByRole('textbox', { name: 'Tags' }).press('Enter');
  await addDialog.getByRole('button', { name: 'Add tags' }).click();
  await expect(page.getByRole('button', { name: 'Remove bulk from first.jpg' })).toBeVisible();

  await page.getByRole('button', { name: 'Remove tags from staged files' }).click();
  const removeDialog = page.getByRole('dialog');
  await removeDialog.getByRole('textbox', { name: 'Tags' }).fill('initial');
  await removeDialog.getByRole('textbox', { name: 'Tags' }).press('Enter');
  await removeDialog.getByRole('button', { name: 'Remove tags' }).click();
  await expect(page.getByRole('button', { name: 'Remove initial from first.jpg' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Remove bulk from first.jpg' })).toBeVisible();

  await page.getByRole('button', { name: 'Remove initial', exact: true }).click();
  await page.getByRole('button', { name: 'Remove future', exact: true }).click();
  await page.getByRole('button', { name: 'SET', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Remove bulk from first.jpg' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Remove future from first.jpg' })).toHaveCount(0);
});
