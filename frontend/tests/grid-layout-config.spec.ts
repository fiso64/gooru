import { expect, test, type Page } from '@playwright/test';

const session = { user: { id: 'usr_test', username: 'mac', role: 'admin' }, capabilities: { upload: true, tag: true, delete: true, admin: true }, csrf_token: 'csrf-one' };
const dimensions = [[1600, 900], [600, 900], [900, 900], [900, 1400], [1200, 800], [700, 1200]] as const;
const thumbnailSizes = [256, 512, 1024];
const retainedFiles = 480;

function fileItem(index: number) {
  const [width, height] = dimensions[index % dimensions.length]; const id = `file-${index}`;
  if (index === 0) {
    return { id, content_id: `hash-${id}`, name: 'issue.cbz', safe_display_path: 'library/issue.cbz', size: 4096,
      added_at: '2026-05-20T00:00:00Z', modified_time: '2026-05-20T00:00:00Z', media_type: 'application/vnd.comicbook+zip', media_kind: 'document',
      metadata: { image_width: 600, image_height: 900, page_count: 24 }, tags: [], media_urls: { thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`, content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download` }, can_delete: false };
  }
  if (index === 3) {
    return { id, content_id: `hash-${id}`, name: 'geom_00073_1200x1600.jpg', safe_display_path: 'library/geom_00073_1200x1600.jpg', size: 2048,
      added_at: '2026-05-20T00:00:00Z', modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
      metadata: {}, tags: [], media_urls: { thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`, content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download` }, can_delete: false };
  }
  return { id, content_id: `hash-${id}`, name: `${id}.jpg`, safe_display_path: `library/${id}.jpg`, size: 2048,
    added_at: '2026-05-20T00:00:00Z', modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
    metadata: { image_width: width, image_height: height }, tags: [], media_urls: { thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`, content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download` }, can_delete: false };
}

async function mockApp(page: Page, gridType: 'square' | 'fit' | 'tile', gridSize = 200) {
  const files = Array.from({ length: retainedFiles }, (_, index) => fileItem(index));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ font_style: 'editorial', grid_size: gridSize, grid_type: gridType, thumbnail_sizes: thumbnailSizes }) }));
  for (const path of ['jobs', 'saved-searches', 'upload-targets']) await page.route(`**/api/v1/${path}`, async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: 10_000, library_count: 10_000, facets: { kind: [{ value: 'photo', count: 10_000 }] }, next_page_token: '480' }) }));
  // Playwright gives later route registrations precedence, so register the generic
  // thumbnail mock first and then layer file-specific derivative shapes on top.
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>' }));
  await page.route('**/api/v1/files/file-0/thumbnail*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="600" height="900"><rect width="600" height="900" fill="#777"/></svg>' }));
  await page.route('**/api/v1/files/file-3/thumbnail*', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="256" height="341"><rect width="256" height="341" fill="#777"/></svg>' }));
  await page.goto('/'); await expect(page.getByText('10,000 files')).toBeVisible();
}

function selectedSize(width: number, height: number, dpr: number, sourceWidth: number, sourceHeight: number, contain = false) {
  let renderedWidth = width; let renderedHeight = height;
  if (contain) {
    const sourceAspect = sourceWidth / sourceHeight; const boxAspect = width / height;
    if (sourceAspect >= boxAspect) renderedHeight = width / sourceAspect;
    else renderedWidth = height * sourceAspect;
  }
  const scale = Math.max(renderedWidth * dpr / sourceWidth, renderedHeight * dpr / sourceHeight);
  const required = Math.min(sourceWidth, sourceHeight) * scale;
  return thumbnailSizes.find((size) => size >= required) ?? thumbnailSizes[thumbnailSizes.length - 1];
}

test('square gallery sizes thumbnails by the derivative short edge', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 }); await mockApp(page, 'square');
  const grid = page.getByTestId('virtual-media-grid'); await expect(grid).toHaveAttribute('data-grid-type', 'square');
  const card = grid.locator('.thumb').nth(4); const box = await card.boundingBox(); const dpr = await page.evaluate(() => window.devicePixelRatio);
  const expectedSize = selectedSize(box?.width ?? 0, box?.height ?? 0, dpr, 1200, 800);
  expect(expectedSize).toBe(256);
  await expect(card.locator('img')).toHaveAttribute('src', new RegExp(`[?&]size=${expectedSize}(?:&|$)`));
});

test('fit gallery keeps square virtual cells and sizes thumbnails against the padded media box', async ({ page }) => {
  await page.setViewportSize({ width: 1080, height: 900 }); await mockApp(page, 'fit');
  const grid = page.getByTestId('virtual-media-grid'); await expect(grid).toHaveAttribute('data-grid-type', 'fit');
  const card = grid.locator('.thumb').nth(2); const box = await card.boundingBox(); expect(box?.width).toBeCloseTo(box?.height ?? 0, 0);
  const open = card.locator('.thumb-open'); await expect(open).toHaveCSS('padding-left', '20px'); await expect(open).toHaveCSS('padding-top', '20px');
  const image = card.locator('img'); await expect(image).toHaveCSS('object-fit', 'contain');
  const dpr = await page.evaluate(() => window.devicePixelRatio);
  const cardWidth = box?.width ?? 0; const cardHeight = box?.height ?? 0;
  const contentWidth = Math.max(1, cardWidth - 40); const contentHeight = Math.max(1, cardHeight - 40);
  const expectedSize = selectedSize(contentWidth, contentHeight, dpr, 900, 900, true);
  const unpaddedSize = selectedSize(cardWidth, cardHeight, dpr, 900, 900, true);
  expect(expectedSize).toBe(256); expect(unpaddedSize).toBe(512);
  await expect(image).toHaveAttribute('src', new RegExp(`[?&]size=${expectedSize}(?:&|$)`));
});

test('tile gallery uses CBZ cover metadata for aspect ratio and thumbnail sizing while keeping a bounded DOM', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 }); await mockApp(page, 'tile');
  const grid = page.getByTestId('virtual-media-grid'); await expect(grid).toHaveAttribute('data-grid-type', 'tile');
  const cards = grid.locator('.virtual-media-item'); await expect.poll(() => cards.count()).toBeGreaterThan(0);
  expect(await cards.count()).toBeLessThan(160); expect(await cards.count()).toBeLessThan(retainedFiles / 2);
  const comic = cards.first();
  await expect(comic.locator('.thumb-badge-extension')).toHaveText('CBZ');
  await expect(comic.locator('img')).toHaveCSS('object-fit', 'contain');
  const comicBox = await comic.boundingBox();
  expect((comicBox?.width ?? 0) / (comicBox?.height ?? 1)).toBeCloseTo(600 / 900, 1);
  const comicImageBox = await comic.locator('img').boundingBox();
  expect((comicImageBox?.height ?? 0) <= (comicBox?.height ?? 0)).toBe(true);
  expect((comicImageBox?.width ?? 0) <= (comicBox?.width ?? 0)).toBe(true);
  const secondBox = await cards.nth(1).boundingBox();
  expect((secondBox?.width ?? 0) / (secondBox?.height ?? 1)).toBeCloseTo(600 / 900, 1);
  const dpr = await page.evaluate(() => window.devicePixelRatio);
  await expect(comic.locator('img')).toHaveAttribute('src', new RegExp(`[?&]size=${selectedSize(comicBox?.width ?? 0, comicBox?.height ?? 0, dpr, 600, 900, true)}(?:&|$)`));
  await expect(cards.nth(1).locator('img')).toHaveAttribute('src', new RegExp(`[?&]size=${selectedSize(secondBox?.width ?? 0, secondBox?.height ?? 0, dpr, 600, 900, true)}(?:&|$)`));

  const missingDimensions = grid.locator('.virtual-media-item').filter({ has: page.getByAltText('geom_00073_1200x1600.jpg') });
  await expect(missingDimensions).toBeVisible();
  await expect.poll(async () => {
    const box = await missingDimensions.boundingBox();
    return (box?.width ?? 0) / (box?.height ?? 1);
  }).toBeCloseTo(256 / 341, 1);

  await page.locator('.main').evaluate((node) => { node.scrollTop = 6000; node.dispatchEvent(new Event('scroll')); }); await page.waitForTimeout(100);
  expect(await cards.count()).toBeLessThan(160); expect(await cards.count()).toBeLessThan(retainedFiles / 2);
});
