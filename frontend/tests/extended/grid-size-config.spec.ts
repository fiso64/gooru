import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockApp(page: Page, gridSize: number, dimensions = { width: 800, height: 600 }) {
  const files = Array.from({ length: 8 }, (_, index) => ({
    id: `file-${index}`,
    content_id: `hash-${index}`,
    name: `file-${index}.jpg`,
    safe_display_path: `library/file-${index}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: dimensions.width, image_height: dimensions.height },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/file-${index}/thumbnail`,
      preview: `/api/v1/files/file-${index}/preview`,
      content: `/api/v1/files/file-${index}/content`,
      download: `/api/v1/files/file-${index}/download`
    }
  }));
  let loggedIn = false;

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      font_style: 'editorial',
      load_full_media_by_default: false,
      grid_size: gridSize,
      thumbnail_sizes: [512, 256, 512]
    })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#fff"/></svg>'
  }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

async function expectSquareThumbnailMatchesCard(page: Page) {
  const card = page.getByTestId('virtual-media-grid').locator('.thumb').first();
  const image = card.locator('img');
  const box = await card.boundingBox();
  const expectedSize = (box?.width ?? 0) <= 256 ? 256 : 512;
  await expect(image).toHaveAttribute('src', new RegExp(`[?&]size=${expectedSize}(?:&|$)`));
}

test('runtime ui grid_size changes the fluid media grid and virtualization together', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  await mockApp(page, 240);

  const rootGridCell = await page.locator('.gooru-root').evaluate((node) => getComputedStyle(node).getPropertyValue('--grid-cell').trim());
  expect(rootGridCell).toBe('240px');

  const grid = page.getByTestId('virtual-media-grid');
  const columns = await grid.evaluate((node) => getComputedStyle(node).gridTemplateColumns.split(' ').length);
  expect(columns).toBe(3);

  const cards = grid.locator('.thumb');
  await expect(cards).toHaveCount(8);
  const firstBox = await cards.first().boundingBox();
  expect(firstBox?.width).toBeGreaterThanOrEqual(240);

  await expect(cards.first().locator('img')).toHaveAttribute('src', /[?&]size=512(?:&|$)/);
});

test('thumbnail sizing follows shared grid geometry after responsive resize', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  await mockApp(page, 180, { width: 800, height: 800 });

  await expectSquareThumbnailMatchesCard(page);
  await page.setViewportSize({ width: 880, height: 900 });
  await expectSquareThumbnailMatchesCard(page);
});

test('thumbnail sizing covers the square card short edge for portrait media', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  await mockApp(page, 180, { width: 600, height: 900 });

  const card = page.getByTestId('virtual-media-grid').locator('.thumb').first();
  const box = await card.boundingBox();
  expect(box?.width).toBeGreaterThan(170);
  expect(box?.width).toBeLessThan(256);
  expect((box?.width ?? 0) * 1.5).toBeGreaterThan(256);

  // A 256 max-edge derivative of a 2:3 portrait is only ~171px wide and
  // would be upscaled by object-fit: cover. The 512 derivative covers it.
  await expect(card.locator('img')).toHaveAttribute('src', /[?&]size=512(?:&|$)/);
});
