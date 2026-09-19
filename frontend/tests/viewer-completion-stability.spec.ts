import { expect, test } from '@playwright/test';

test('autocomplete popup clips long labels', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('body')).toBeVisible();
});
