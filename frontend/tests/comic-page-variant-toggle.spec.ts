import { expect, test } from '@playwright/test';
import { setupComicVariantFixture } from './comic-variant-fixture';

test('comic page preview and lossless source toggle', async ({ page }) => {
  await setupComicVariantFixture(page);
  await page.getByRole('button', { name: 'Preview book.cbz' }).click();
  const dialog = page.getByRole('dialog', { name: 'book.cbz' });
  await dialog.getByRole('button', { name: 'Read comic' }).click();
  const image = dialog.locator('img.viewer-visual-media');
  await expect(image).toHaveAttribute('src', '/api/v1/comics/book/0?variant=preview');
  await dialog.getByRole('button', { name: 'Use original media' }).click();
  await expect(image).toHaveAttribute('src', '/api/v1/comics/book/0?variant=lossless');
});
