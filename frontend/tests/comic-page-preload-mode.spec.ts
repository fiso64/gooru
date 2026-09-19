import { expect, test } from '@playwright/test';
import { setupComicVariantFixture } from './comic-variant-fixture';

test('comic preloading follows full-image mode after navigation', async ({ page }) => {
  await setupComicVariantFixture(page);
  await page.getByRole('button', { name: 'Preview book.cbz' }).click();
  const dialog = page.getByRole('dialog', { name: 'book.cbz' });
  await dialog.getByRole('button', { name: 'Read comic' }).click();
  await expect(dialog.locator('img.viewer-visual-media')).toBeVisible();
  await dialog.getByRole('button', { name: 'Use original media' }).click();
  const preload = page.waitForRequest(request => request.url().endsWith('/api/v1/comics/book/2?variant=lossless'));
  await dialog.getByRole('button', { name: 'Next page' }).click();
  await expect(dialog.locator('img.viewer-visual-media')).toHaveAttribute('src', '/api/v1/comics/book/1?variant=lossless');
  await preload;
});
