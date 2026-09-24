import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(index: number) {
  const id = `file-${index}`;
  return {
    id,
    content_id: `hash-${id}`,
    name: `selection-${String(index).padStart(3, '0')}.jpg`,
    safe_display_path: `library/${id}.jpg`,
    size: 2048,
    modified_time: '2026-05-20T00:00:00Z',
    media_type: 'image/jpeg',
    media_kind: 'photo',
    metadata: { image_width: 800, image_height: 600 },
    tags: [],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    }
  };
}

async function mockApp(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) });
  });
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  const files = Array.from({ length: 120 }, (_, index) => fileItem(index));
  await page.route('**/api/v1/files?**', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [{ value: 'photo', count: files.length }] } })
    });
  });
  await page.route('**/api/v1/files/*/thumbnail', async (route) => {
    await route.fulfill({ contentType: 'image/svg+xml', body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>' });
  });
}

test('selection controls avoid per-card backdrop filters while scrolling', async ({ page }) => {
  await mockApp(page);
  await page.goto('/');
  await expect(page.getByText('120 files')).toBeVisible();

  const visibleCards = page.locator('.thumb');
  await expect.poll(() => visibleCards.count()).toBeLessThan(120);
  const initialVisible = await visibleCards.count();
  expect(initialVisible).toBeGreaterThan(2);

  const firstCheckbox = visibleCards.nth(0).locator('.thumb-checkbox');
  await firstCheckbox.click();
  const secondCard = visibleCards.nth(1);
  await secondCard.locator('.thumb-open').click();

  const selectedCheckboxes = page.locator('.thumb-checkbox.is-selected');
  await expect(selectedCheckboxes).toHaveCount(2);
  for (let index = 0; index < 2; index += 1) {
    const filters = await selectedCheckboxes.nth(index).evaluate((node) => {
      const style = getComputedStyle(node);
      return { backdropFilter: style.backdropFilter, webkitBackdropFilter: (style as CSSStyleDeclaration & { webkitBackdropFilter?: string }).webkitBackdropFilter || 'none' };
    });
    expect(filters.backdropFilter).toBe('none');
    expect(filters.webkitBackdropFilter).toBe('none');
  }

  const preview = secondCard.locator('.thumb-preview');
  await expect(preview).toBeVisible();
  expect(await preview.evaluate((node) => getComputedStyle(node).backdropFilter)).toBe('none');
  expect(await secondCard.evaluate((node) => getComputedStyle(node).contain)).toBe('content');

  await page.locator('.main').evaluate((node) => {
    node.scrollTop = 8000;
    node.dispatchEvent(new Event('scroll'));
  });
  await expect.poll(() => page.locator('.thumb').count()).toBeLessThan(120);
});
