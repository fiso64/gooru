import { expect, test } from '@playwright/test';

test('viewer page variant regression fixture', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('body')).toBeVisible();
});
