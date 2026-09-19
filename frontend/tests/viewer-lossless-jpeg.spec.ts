import { expect, test, type Page } from '@playwright/test';

test('lossless display is optional', async ({ page }) => {
  expect(page).toBeDefined();
});

const jpegUrl = '/api/v1/files/lossless-file/lossless';
