import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const aspectDimensions = [[1600, 900], [600, 900], [900, 900], [900, 1400], [1200, 800], [700, 1200], [2000, 800]] as const;

function fileItem(index: number, dimensions: readonly [number, number] = aspectDimensions[index % aspectDimensions.length]) {
  const id = `file-${index}`;
  const [width, height] = dimensions;
  return {
    id, content_id: `hash-${id}`, name: `perf-${index}.jpg`, safe_display_path: `library/${id}.jpg`,
    size: 2048, modified_time: '2026-05-20T00:00:00Z', media_type: index % 2 ? 'video/mp4' : 'image/jpeg', media_kind: index % 2 ? 'video' : 'photo',
    metadata: { image_width: width, image_height: height }, tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`
    }
  };
}

function pagedFileItem(index: number) {
  const pageIndex = index % 60;
  // Force every transport page to end with an unmistakably incomplete tile row regardless
  // of the exact browser grid width. Item 56 is wide enough to close the preceding row;
  // items 57-59 are tiny portraits whose combined target width cannot close a normal row.
  if (pageIndex === 56) return fileItem(index, [8000, 1000]);
  if (pageIndex >= 57) return fileItem(index, [125, 1000]);
  return fileItem(index);
}

async function mockApp(page: Page, gridType: 'square' | 'tile' = 'square') {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ grid_type: gridType }) }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  const files = Array.from({ length: 600 }, (_, index) => fileItem(index));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: 10_000, library_count: 10_000, facets: { kind: [{ value: 'photo', count: 10_000 }] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));
}

async function mockPagedApp(page: Page, gridType: 'square' | 'fit' | 'tile', gridSize: number, holdOffset = 60) {
  const allFiles = Array.from({ length: 600 }, (_, index) => fileItem(index));
  const requestedOffsets: number[] = [];
  let releaseHeldPage!: () => void;
  const heldPageGate = new Promise<void>((resolve) => { releaseHeldPage = resolve; });

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ grid_type: gridType, grid_size: gridSize }) }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('page_token') ?? '0');
    requestedOffsets.push(offset);
    if (offset === holdOffset) await heldPageGate;
    const files = allFiles.slice(offset, offset + 60);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files,
        total_count: allFiles.length,
        library_count: allFiles.length,
        facets: offset === 0 ? { kind: [{ value: 'photo', count: allFiles.length }] } : undefined,
        next_page_token: offset + 60 < allFiles.length ? String(offset + 60) : undefined,
        previous_page_token: offset > 0 ? String(Math.max(0, offset - 60)) : undefined
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));

  return { requestedOffsets, releaseHeldPage };
}

async function mockPagedTileApp(page: Page) {
  const allFiles = Array.from({ length: 1200 }, (_, index) => pagedFileItem(index));
  const requestedOffsets: number[] = [];
  let releaseNinthPage!: () => void;
  const ninthPageGate = new Promise<void>((resolve) => { releaseNinthPage = resolve; });

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ grid_type: 'tile', grid_size: 200 }) }));
  await page.route('**/api/v1/jobs', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('page_token') ?? '0');
    requestedOffsets.push(offset);
    if (offset === 480) await ninthPageGate;
    const files = allFiles.slice(offset, offset + 60);
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({
        files,
        total_count: allFiles.length,
        library_count: allFiles.length,
        facets: offset === 0 ? { kind: [{ value: 'photo', count: allFiles.length }] } : undefined,
        next_page_token: offset + 60 < allFiles.length ? String(offset + 60) : undefined,
        previous_page_token: offset > 0 ? String(Math.max(0, offset - 60)) : undefined
      })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));

  return { requestedOffsets, releaseNinthPage };
}

test('large library keeps a bounded DOM while sustained scrolling advances the virtual window', async ({ page }) => {
  await mockApp(page);
  await page.goto('/');
  await expect(page.getByText('10,000 files')).toBeVisible();
  const cards = page.locator('.thumb');
  await expect.poll(() => cards.count()).toBeLessThan(100);
  const badge = page.locator('.thumb-badge').first();
  await expect(badge).toBeVisible();
  expect(await badge.evaluate((node) => getComputedStyle(node).backdropFilter)).toBe('none');

  const samples = await page.locator('.main').evaluate(async (node) => {
    const grid = node.querySelector<HTMLElement>('[data-testid="virtual-media-grid"]')!;
    const values: string[] = [];
    for (let step = 1; step <= 30; step += 1) {
      node.scrollTop = step * 250;
      node.dispatchEvent(new Event('scroll'));
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
      values.push(grid.style.transform);
    }
    return values;
  });
  expect(new Set(samples).size).toBeGreaterThan(5);
  expect(samples.at(-1)).not.toBe(samples[0]);
  expect(await cards.count()).toBeLessThan(100);
});

for (const scenario of [
  { gridType: 'square' as const, gridSize: 120, width: 1050 },
  { gridType: 'fit' as const, gridSize: 220, width: 1180 },
  { gridType: 'tile' as const, gridSize: 120, width: 1050 },
  { gridType: 'tile' as const, gridSize: 280, width: 1440 }
]) {
  test(`${scenario.gridType} grid ${scenario.gridSize}px keeps viewport anchor stable across a page append`, async ({ page }) => {
    await page.setViewportSize({ width: scenario.width, height: 820 });
    const paging = await mockPagedApp(page, scenario.gridType, scenario.gridSize);
    await page.goto('/');
    await expect(page.getByText('600 files')).toBeVisible();
    const main = page.locator('.main');

    // Move close enough to the end of the first loaded page to request the next page while
    // retaining a first-page card in the visible window. This models the real page-turn
    // boundary rather than asserting only internal geometry coordinates.
    for (let scrollTop = 300; scrollTop <= 20_000 && !paging.requestedOffsets.includes(60); scrollTop += 250) {
      await main.evaluate((node, y) => { node.scrollTop = y; node.dispatchEvent(new Event('scroll')); }, scrollTop);
      await page.waitForTimeout(16);
    }
    await expect.poll(() => paging.requestedOffsets.includes(60)).toBe(true);

    const visible = page.locator('[data-testid="virtual-media-grid"] .thumb-open').filter({ visible: true });
    const count = await visible.count();
    expect(count).toBeGreaterThan(0);
    const anchor = visible.nth(Math.floor(count / 2));
    const anchorLabel = await anchor.getAttribute('aria-label');
    expect(anchorLabel).toBeTruthy();
    const beforeBox = await anchor.boundingBox();
    expect(beforeBox).not.toBeNull();
    const beforeScroll = await main.evaluate((node) => node.scrollTop);

    paging.releaseHeldPage();
    await expect.poll(() => paging.requestedOffsets.filter((offset) => offset === 60).length).toBe(1);
    await expect.poll(async () => page.locator('[data-testid="virtual-media-grid"] .thumb-open').count()).toBeGreaterThan(count);
    await page.waitForTimeout(100);

    const sameAnchor = page.getByRole('button', { name: anchorLabel! });
    await expect(sameAnchor).toBeVisible();
    const afterBox = await sameAnchor.boundingBox();
    expect(afterBox).not.toBeNull();
    const afterScroll = await main.evaluate((node) => node.scrollTop);

    expect(afterScroll).toBeCloseTo(beforeScroll, 1);
    expect(afterBox!.x).toBeCloseTo(beforeBox!.x, 1);
    expect(afterBox!.y).toBeCloseTo(beforeBox!.y, 1);
  });
}

test('tile pagination keeps committed rows stable and does not publish an incomplete page tail', async ({ page }) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  const paging = await mockPagedTileApp(page);
  await page.goto('/');
  await expect(page.getByText('1,200 files')).toBeVisible();
  const main = page.locator('.main');
  const grid = page.getByTestId('virtual-media-grid');
  const stableTarget = grid.getByRole('button', { name: 'Preview perf-476.jpg' });
  const pendingTarget = grid.getByRole('button', { name: 'Preview perf-478.jpg' });

  // Page eight deliberately ends with items 477-479 unable to close a tile row. Hold page
  // nine so the transport-page tail is observable: it must remain unpublished rather than
  // being rendered as a temporary final row and then repacked on append.
  for (let scrollTop = 3000; scrollTop <= 50_000 && !paging.requestedOffsets.includes(480); scrollTop += 2000) {
    await main.evaluate((node, y) => { node.scrollTop = y; node.dispatchEvent(new Event('scroll')); }, scrollTop);
    await page.waitForTimeout(30);
  }
  await expect.poll(() => paging.requestedOffsets.includes(480)).toBe(true);

  for (let scrollTop = 20_000; scrollTop <= 50_000 && await stableTarget.count() === 0; scrollTop += 500) {
    await main.evaluate((node, y) => { node.scrollTop = y; node.dispatchEvent(new Event('scroll')); }, scrollTop);
    await page.waitForTimeout(16);
  }
  await expect(stableTarget).toBeVisible();
  await expect(pendingTarget).toHaveCount(0);
  const before = await stableTarget.boundingBox();
  expect(before).not.toBeNull();

  paging.releaseNinthPage();
  await expect.poll(() => paging.requestedOffsets.filter((offset) => offset === 480).length).toBe(1);
  await page.waitForTimeout(100);
  await expect(stableTarget).toBeVisible();
  const after = await stableTarget.boundingBox();
  expect(after).not.toBeNull();
  expect(after!.x).toBeCloseTo(before!.x, 1);
  expect(after!.y).toBeCloseTo(before!.y, 1);
  expect(await page.locator('.virtual-media-item').count()).toBeLessThan(160);
});
