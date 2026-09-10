import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const aspects = [[1600, 900], [600, 900], [900, 900], [900, 1400], [1200, 800], [700, 1200], [2000, 800]] as const;

function fileItem(index: number) {
  const id = `file-${index}`;
  const [width, height] = aspects[index % aspects.length];
  return {
    id, content_id: `hash-${id}`, name: `perf-${index}.jpg`, safe_display_path: `library/${id}.jpg`,
    size: 2048, modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
    metadata: { image_width: width, image_height: height }, tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockPagedApp(page: Page, gridType: 'square' | 'fit' | 'tile', gridSize: number) {
  const allFiles = Array.from({ length: 600 }, (_, index) => fileItem(index));
  const requestedOffsets: number[] = [];
  const fulfilledOffsets: number[] = [];
  let releaseSecondPage!: () => void;
  const secondPageGate = new Promise<void>((resolve) => { releaseSecondPage = resolve; });

  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ grid_type: gridType, grid_size: gridSize }) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => {
    const url = new URL(route.request().url());
    const offset = Number(url.searchParams.get('page_token') ?? '0');
    requestedOffsets.push(offset);
    if (offset === 60) await secondPageGate;
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
    fulfilledOffsets.push(offset);
  });
  await page.route('**/api/v1/files/*/thumbnail*', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));

  return { requestedOffsets, fulfilledOffsets, releaseSecondPage };
}

for (const scenario of [
  { gridType: 'square' as const, gridSize: 120, width: 1050, effectiveSize: 120 },
  { gridType: 'fit' as const, gridSize: 220, width: 1180, effectiveSize: 260 },
  { gridType: 'tile' as const, gridSize: 120, width: 1050, effectiveSize: 160 },
  { gridType: 'tile' as const, gridSize: 280, width: 1440, effectiveSize: 320 }
]) {
  test(`${scenario.gridType} ${scenario.gridSize}px keeps viewport anchor stable across page append`, async ({ page }) => {
    await page.setViewportSize({ width: scenario.width, height: 820 });
    const paging = await mockPagedApp(page, scenario.gridType, scenario.gridSize);
    await page.goto('/');
    await expect(page.getByText('600 files')).toBeVisible();
    const rootGridCell = await page.locator('.gooru-root').evaluate((node) => getComputedStyle(node).getPropertyValue('--grid-cell').trim());
    expect(rootGridCell).toBe(`${scenario.effectiveSize}px`);
    const main = page.locator('.main');

    for (let scrollTop = 300; scrollTop <= 20_000 && !paging.requestedOffsets.includes(60); scrollTop += 250) {
      await main.evaluate((node, y) => { node.scrollTop = y; node.dispatchEvent(new Event('scroll')); }, scrollTop);
      await page.waitForTimeout(16);
    }
    await expect.poll(() => paging.requestedOffsets.includes(60)).toBe(true);

    const visible = page.locator('[data-testid="virtual-media-grid"] .thumb-open:visible');
    const count = await visible.count();
    expect(count).toBeGreaterThan(0);
    const anchor = visible.nth(Math.floor(count / 2));
    const anchorLabel = await anchor.getAttribute('aria-label');
    expect(anchorLabel).toBeTruthy();
    const beforeBox = await anchor.boundingBox();
    expect(beforeBox).not.toBeNull();
    const beforeScroll = await main.evaluate((node) => node.scrollTop);

    paging.releaseSecondPage();
    await expect.poll(() => paging.fulfilledOffsets.includes(60)).toBe(true);
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

test('square virtual windows begin on the same row boundary the browser renders', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 820 });
  await mockPagedApp(page, 'square', 200);
  await page.goto('/');
  await expect(page.getByText('600 files')).toBeVisible();

  const main = page.locator('.main');
  await main.evaluate((node) => { node.scrollTop = 2500; node.dispatchEvent(new Event('scroll')); });
  await page.waitForTimeout(50);

  const grid = page.locator('[data-testid="virtual-media-grid"]');
  const first = grid.locator('.thumb-open').first();
  const label = await first.getAttribute('aria-label');
  expect(label).toBeTruthy();
  const match = label!.match(/perf-(\d+)\.jpg$/);
  expect(match).not.toBeNull();
  const firstIndex = Number(match![1]);
  expect(firstIndex).toBeGreaterThan(0);

  const renderedColumns = await grid.evaluate((node) => {
    const template = getComputedStyle(node).gridTemplateColumns.trim();
    return template ? template.split(/\s+/).length : 0;
  });
  expect(renderedColumns).toBeGreaterThan(0);
  expect(firstIndex % renderedColumns).toBe(0);
});
