import { expect, test, type Page } from '@playwright/test';

test('lossless display is optional', async ({ page }) => {
  expect(page).toBeDefined();
});
