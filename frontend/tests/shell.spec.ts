import { expect, test } from '@playwright/test';

test('renders the library shell', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await expect(page.getByPlaceholder('Paste server token')).toBeVisible();
  await expect(page.getByPlaceholder('tag, key:value, @tagged')).toBeVisible();
});
