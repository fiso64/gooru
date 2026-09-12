import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function imageItem() {
  return {
    id: 'large-image',
    content_id: 'hash-large-image',
    name: 'large.jpg',
    safe_display_path: 'library/large.jpg',
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'image',
    metadata: { image_width: 2400, image_height: 1600 },
    tags: [],
    media_urls: {
      thumbnail: '/api/v1/files/large-image/thumbnail',
      preview: '/api/v1/files/large-image/preview',
      content: '/api/v1/files/large-image/content',
      download: '/api/v1/files/large-image/download'
    }
  };
}

async function mockViewer(page: Page) {
  let loggedIn = false;
  const files = [imageItem()];
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    status: loggedIn ? 200 : 401,
    contentType: 'application/json',
    body: JSON.stringify(loggedIn ? session : { error: { code: 'unauthorized', message: 'login required' } })
  }));
  await page.route('**/api/v1/auth/login', async (route) => {
    loggedIn = true;
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ files, total_count: 1, library_count: 1, facets: { kind: [] } }) }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="2400" height="1600"><rect width="2400" height="1600"/></svg>';
  await page.route('**/api/v1/files/large-image/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/large-image/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/large-image/content', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await page.getByLabel('Username').fill('mac');
  await page.getByLabel('Password').fill('correct horse');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  await page.getByRole('button', { name: 'Preview large.jpg' }).click();
  const image = page.locator('img.viewer-visual-media');
  await expect(image).toBeVisible();
  return image;
}

async function wheelWithModifier(page: Page, key: 'Control' | 'Alt', deltaY: number) {
  await page.keyboard.down(key);
  await page.mouse.wheel(0, deltaY);
  await page.keyboard.up(key);
}

test('touchpad-sized zoom and pan input applies immediately without transform easing', async ({ page }) => {
  const image = await mockViewer(page);
  const stage = page.locator('.viewer-stage');
  const panViewport = page.locator('.viewer-pan-viewport');
  const stageBox = await stage.boundingBox();
  const before = await image.boundingBox();
  expect(stageBox).not.toBeNull();
  expect(before).not.toBeNull();

  const transitionProperties = await image.evaluate((element) => getComputedStyle(element).transitionProperty.split(',').map((value) => value.trim()));
  expect(transitionProperties).not.toContain('transform');

  await page.mouse.move(stageBox!.x + stageBox!.width / 2, stageBox!.y + stageBox!.height / 2);
  await wheelWithModifier(page, 'Control', -24);
  const touchpadZoomed = await image.boundingBox();
  expect(touchpadZoomed).not.toBeNull();
  expect(touchpadZoomed!.width).toBeGreaterThan(before!.width * 1.15);

  // Establish enough vertical overflow to exercise panning. The small pan delta below is
  // still touchpad-sized; without overflow the viewer correctly clamps it to zero.
  await wheelWithModifier(page, 'Control', -160);
  const zoomedForPan = await image.boundingBox();
  expect(zoomedForPan).not.toBeNull();
  expect(zoomedForPan!.height).toBeGreaterThan(stageBox!.height + 40);

  const beforeVerticalScroll = await panViewport.evaluate((node) => node.scrollTop);
  await page.mouse.wheel(0, 24);
  await expect.poll(() => panViewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(beforeVerticalScroll + 15);
  const panned = await image.boundingBox();
  expect(panned).not.toBeNull();
  expect(panned!.y).toBeLessThan(zoomedForPan!.y - 15);
});

test('ctrl-wheel zooms toward the pointer and wheel/alt-wheel pan while zoomed', async ({ page }) => {
  const image = await mockViewer(page);
  const stage = page.locator('.viewer-stage');
  const panViewport = page.locator('.viewer-pan-viewport');
  const stageBox = await stage.boundingBox();
  const before = await image.boundingBox();
  expect(stageBox).not.toBeNull();
  expect(before).not.toBeNull();

  const cursorX = stageBox!.x + stageBox!.width * 0.7;
  const cursorY = stageBox!.y + stageBox!.height * 0.4;
  await page.mouse.move(cursorX, cursorY);
  await wheelWithModifier(page, 'Control', -360);
  await page.waitForTimeout(160);
  const zoomed = await image.boundingBox();
  expect(zoomed).not.toBeNull();
  expect(zoomed!.width).toBeGreaterThan(before!.width * 1.5);
  const beforeU = (cursorX - before!.x) / before!.width;
  const beforeV = (cursorY - before!.y) / before!.height;
  expect(Math.abs((zoomed!.x + beforeU * zoomed!.width) - cursorX)).toBeLessThan(3);
  expect(Math.abs((zoomed!.y + beforeV * zoomed!.height) - cursorY)).toBeLessThan(3);

  const beforeVerticalScroll = await panViewport.evaluate((node) => node.scrollTop);
  await page.mouse.wheel(0, 120);
  await expect.poll(() => panViewport.evaluate((node) => node.scrollTop)).toBeGreaterThan(beforeVerticalScroll + 80);
  await page.waitForTimeout(40);
  const verticallyPanned = await image.boundingBox();
  expect(verticallyPanned!.y).toBeLessThan(zoomed!.y - 20);

  const beforeHorizontalScroll = await panViewport.evaluate((node) => node.scrollLeft);
  await wheelWithModifier(page, 'Alt', 120);
  await expect.poll(() => panViewport.evaluate((node) => node.scrollLeft)).toBeGreaterThan(beforeHorizontalScroll + 80);
  await page.waitForTimeout(40);
  const horizontallyPanned = await image.boundingBox();
  expect(horizontallyPanned!.x).toBeLessThan(verticallyPanned!.x - 20);
});

test('zoom minima respect fit and actual-size modes across normal and fullscreen geometry', async ({ page }) => {
  const image = await mockViewer(page);
  const stage = page.locator('.viewer-stage');
  const fit = await image.boundingBox();
  expect(fit).not.toBeNull();

  const box = await stage.boundingBox();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await wheelWithModifier(page, 'Control', 2000);
  await page.waitForTimeout(160);
  const fitMinimum = await image.boundingBox();
  expect(Math.abs(fitMinimum!.width - fit!.width)).toBeLessThan(2);

  await stage.focus();
  await page.keyboard.press('2');
  await page.waitForTimeout(160);
  const actual = await image.boundingBox();
  expect(actual!.width).toBeGreaterThan(fit!.width * 1.5);
  await wheelWithModifier(page, 'Control', 4000);
  await page.waitForTimeout(160);
  const actualMinimum = await image.boundingBox();
  expect(actualMinimum!.width).toBeGreaterThanOrEqual(fit!.width - 2);
  expect(actualMinimum!.width).toBeLessThan(actual!.width);

  // Entering fullscreen increases the fit scale for this image. The carried actual-size
  // transform must be raised to the new fullscreen minimum instead of staying undersized.
  await page.keyboard.press('f');
  await expect.poll(() => stage.evaluate((node) => document.fullscreenElement === node)).toBe(true);
  const fullscreenStageFromActual = await stage.boundingBox();
  const expectedFullscreenFitWidth = Math.min(fullscreenStageFromActual!.width, fullscreenStageFromActual!.height * 1.5);
  await expect.poll(async () => (await image.boundingBox())?.width ?? 0).toBeGreaterThanOrEqual(expectedFullscreenFitWidth - 2);

  await page.keyboard.press('f');
  await expect.poll(() => stage.evaluate((node) => document.fullscreenElement === node)).toBe(false);
  await page.keyboard.press('1');
  await page.keyboard.press('f');
  await expect.poll(() => stage.evaluate((node) => document.fullscreenElement === node)).toBe(true);
  const fullscreenFit = await image.boundingBox();
  const fullscreenStage = await stage.boundingBox();
  await page.mouse.move(fullscreenStage!.x + fullscreenStage!.width * 0.65, fullscreenStage!.y + fullscreenStage!.height * 0.45);
  await wheelWithModifier(page, 'Control', -300);
  await page.waitForTimeout(160);
  const fullscreenZoomed = await image.boundingBox();
  expect(fullscreenZoomed!.width).toBeGreaterThan(fullscreenFit!.width * 1.4);
});

test('dragging a zoomed image shows a hand cursor only while dragging and pans it', async ({ page }) => {
  const image = await mockViewer(page);
  const stage = page.locator('.viewer-stage');
  const panViewport = page.locator('.viewer-pan-viewport');
  const stageBox = await stage.boundingBox();
  expect(stageBox).not.toBeNull();

  const centerX = stageBox!.x + stageBox!.width / 2;
  const centerY = stageBox!.y + stageBox!.height / 2;
  await page.mouse.move(centerX, centerY);
  await wheelWithModifier(page, 'Control', -220);
  await expect(panViewport).toHaveCSS('cursor', 'default');
  await expect(image).toHaveAttribute('draggable', 'false');

  const beforeScroll = await panViewport.evaluate((node) => ({ left: node.scrollLeft, top: node.scrollTop }));
  expect(beforeScroll.left).toBeGreaterThan(100);
  expect(beforeScroll.top).toBeGreaterThan(100);
  const beforeImage = await image.boundingBox();
  expect(beforeImage).not.toBeNull();

  await page.mouse.down();
  await expect(panViewport).toHaveCSS('cursor', 'grabbing');
  await page.mouse.move(centerX + 120, centerY + 80, { steps: 4 });
  await expect.poll(() => panViewport.evaluate((node) => node.scrollLeft)).toBeLessThan(beforeScroll.left - 80);
  await expect.poll(() => panViewport.evaluate((node) => node.scrollTop)).toBeLessThan(beforeScroll.top - 50);

  const draggedImage = await image.boundingBox();
  expect(draggedImage).not.toBeNull();
  expect(draggedImage!.x).toBeGreaterThan(beforeImage!.x + 80);
  expect(draggedImage!.y).toBeGreaterThan(beforeImage!.y + 50);

  await page.mouse.up();
  await expect(panViewport).toHaveCSS('cursor', 'default');
});