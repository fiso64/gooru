import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(index: number) {
  const id = `file-${index}`;
  return {
    id, content_id: `hash-${id}`, name: `perf-${index}.jpg`, safe_display_path: `library/${id}.jpg`,
    size: 2048, modified_time: '2026-05-20T00:00:00Z', media_type: 'image/jpeg', media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 }, tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`, preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`, download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: '{}' }));
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
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));
}

test('large library keeps a bounded DOM while sustained scrolling advances the virtual window', async ({ page }) => {
  await mockApp(page);
  await page.goto('/');
  await expect(page.getByText('10,000 files')).toBeVisible();
  const cards = page.locator('.thumb');
  await expect.poll(() => cards.count()).toBeLessThan(100);

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
  expect(new Set(samples).size).toBeGreaterThan(10);
  expect(await cards.count()).toBeLessThan(100);
});
