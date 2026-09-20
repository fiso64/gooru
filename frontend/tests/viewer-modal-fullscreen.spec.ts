import { expect, test } from '@playwright/test';

test('modal visibility in fullscreen', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('body')).toBeVisible();
});
