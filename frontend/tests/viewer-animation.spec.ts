import { expect, test, type Page } from '@playwright/test';

const svg = (width: number, height: number, color: string) =>
  `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}"><rect width="100%" height="100%" fill="${color}"/></svg>`)}`;

const files = [
  { id: 'one', filename: 'wide.jpg', width: 900, height: 320, color: '#f66' },
  { id: 'two', filename: 'tall.jpg', width: 320, height: 900, color: '#6f6' },
  { id: 'three', filename: 'square.jpg', width: 640, height: 640, color: '#66f' },
  { id: 'four', filename: 'wide-2.jpg', width: 960, height: 360, color: '#fc6' },
  { id: 'five', filename: 'tall-2.jpg', width: 360, height: 960, color: '#6cf' },
  { id: 'six', filename: 'sixth.jpg', width: 700, height: 420, color: '#c6f' }
];

async function mockApp(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({ grid_type: 'square', grid_size: 200, grid_gap: 8, viewer_fit_mode: 'contain' })
  }));
  await page.route('**/api/v1/files**', async (route) => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      files: files.map((entry) => ({
        id: entry.id,
        filename: entry.filename,
        media_kind: 'photo',
        media_type: 'image/jpeg',
        metadata: { image_width: entry.width, image_height: entry.height },
        media_urls: {
          content: `/api/v1/files/${entry.id}/content`,
          preview: `/api/v1/files/${entry.id}/preview`,
          thumbnail: `/api/v1/files/${entry.id}/thumbnail`,
          download: `/api/v1/files/${entry.id}/download`
        }
      })),
      next_cursor: ''
    })
  }));
  for (const entry of files) {
    await page.route(`**/api/v1/files/${entry.id}/{content,preview,thumbnail}`, async (route) => route.fulfill({
      status: 200,
      contentType: 'image/svg+xml',
      body: decodeURIComponent(svg(entry.width, entry.height, entry.color).split(',')[1])
    }));
  }
  await page.goto('/');
}

test('image navigation holds the old frame while the target becomes ready', async ({ page }) => {
  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toBeVisible();
  const before = await media.boundingBox();
  expect(before).not.toBeNull();

  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'tall.jpg' })).toBeVisible();
  await expect.poll(async () => Boolean(await page.locator('.viewer-image-freeze').count())).toBe(true);
  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().height)).toBeGreaterThan(0);
});

test('rapid navigation never exposes zero-sized image geometry', async ({ page }) => {
  await page.addInitScript(() => {
    const state = window as typeof window & { __viewerZeroSizeObserved?: boolean; __viewerSizeObserver?: MutationObserver };
    state.__viewerZeroSizeObserved = false;
    const observe = () => {
      const media = document.querySelector<HTMLElement>('.viewer-visual-media');
      if (!media) return requestAnimationFrame(observe);
      state.__viewerSizeObserver = new MutationObserver(() => {
        const rect = media.getBoundingClientRect();
        if (rect.width <= 0 || rect.height <= 0) state.__viewerZeroSizeObserved = true;
      });
      state.__viewerSizeObserver.observe(media, { attributes: true, attributeFilter: ['src', 'style', 'class'] });
    };
    requestAnimationFrame(observe);
  });

  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  const media = page.locator('.viewer-visual-media');
  await expect.poll(() => media.evaluate((node) => node.getBoundingClientRect().height)).toBeGreaterThan(0);
  expect(await page.evaluate(() => (window as typeof window & { __viewerZeroSizeObserved?: boolean }).__viewerZeroSizeObserved)).toBe(false);
  await page.evaluate(() => {
    const state = window as typeof window & { __viewerSizeObserver?: MutationObserver };
    state.__viewerSizeObserver?.disconnect();
  });
});

test('rapid navigation bounds expensive image predecodes', async ({ page }) => {
  await page.addInitScript(() => {
    const originalDecode = HTMLImageElement.prototype.decode;
    const state = window as typeof window & { __viewerDecodeActive?: number; __viewerDecodeMax?: number };
    state.__viewerDecodeActive = 0;
    state.__viewerDecodeMax = 0;
    HTMLImageElement.prototype.decode = function () {
      if (!this.src.includes('/preview')) return originalDecode ? originalDecode.call(this) : Promise.resolve();
      state.__viewerDecodeActive = (state.__viewerDecodeActive ?? 0) + 1;
      state.__viewerDecodeMax = Math.max(state.__viewerDecodeMax ?? 0, state.__viewerDecodeActive);
      return new Promise<void>((resolve) => {
        setTimeout(() => {
          state.__viewerDecodeActive = Math.max(0, (state.__viewerDecodeActive ?? 1) - 1);
          resolve();
        }, 150);
      });
    };
  });

  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();
  await page.waitForTimeout(250);
  await page.evaluate(() => {
    const state = window as typeof window & { __viewerDecodeActive?: number; __viewerDecodeMax?: number };
    state.__viewerDecodeActive = 0;
    state.__viewerDecodeMax = 0;
  });

  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'sixth.jpg' })).toBeVisible();
  await page.waitForTimeout(200);

  // Latest-wins navigation cancels speculative work on every move and only primes one
  // direction-aware neighbor after the committed target presents, so image decode concurrency
  // should never exceed one during this burst.
  expect(await page.evaluate(() => (window as typeof window & { __viewerDecodeMax?: number }).__viewerDecodeMax ?? 0)).toBeLessThanOrEqual(1);
});

test('held navigation keeps image presentation advancing when full image decode is backlogged', async ({ page }) => {
  await page.addInitScript(() => {
    const originalDecode = HTMLImageElement.prototype.decode;
    HTMLImageElement.prototype.decode = function () {
      if (!this.src.includes('/preview')) return originalDecode ? originalDecode.call(this) : Promise.resolve();
      return new Promise<void>(() => {});
    };
  });

  await mockApp(page);
  await page.getByRole('button', { name: 'Preview wide.jpg' }).click();

  const media = page.locator('.viewer-visual-media');
  await expect(media).toBeVisible();
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('dialog', { name: 'wide-2.jpg' })).toBeVisible();
  await expect.poll(() => media.getAttribute('src')).toContain('/four/');
});
