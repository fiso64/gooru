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
    name: `range-${index}.jpg`,
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
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify(session) }));
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [{ id: 'default', name: 'Default' }] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ tags: [] }) }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  const files = Array.from({ length: 6 }, (_, index) => fileItem(index));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files, total_count: files.length, library_count: files.length, facets: { kind: [{ value: 'photo', count: files.length }] } })
  }));
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({
    contentType: 'image/svg+xml',
    body: '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><rect width="32" height="32" fill="#777"/></svg>'
  }));
}

test('shift-click selects and deselects ranges from the most recent selection anchor', async ({ page }) => {
  await mockApp(page);
  await page.goto('/');
  const cards = page.locator('.thumb');
  await expect(cards).toHaveCount(6);

  await cards.nth(0).hover();
  await cards.nth(0).locator('.thumb-checkbox').click();
  await expect(page.locator('.thumb-checkbox.is-selected')).toHaveCount(1);

  const hiddenPreview = cards.nth(1).locator('.thumb-preview');
  await expect(hiddenPreview).toBeAttached();
  await expect(hiddenPreview).toHaveCSS('opacity', '0');
  await cards.nth(1).hover();
  await expect(hiddenPreview).toHaveCSS('opacity', '1');

  await cards.nth(3).locator('.thumb-open').click({ modifiers: ['Shift'] });
  await expect(page.locator('.thumb-checkbox.is-selected')).toHaveCount(4);

  await cards.nth(1).locator('.thumb-open').click({ modifiers: ['Shift'] });
  await expect(page.locator('.thumb-checkbox.is-selected')).toHaveCount(1);
  await expect(cards.nth(0).locator('.thumb-checkbox')).toHaveClass(/is-selected/);

  await cards.nth(4).locator('.thumb-open').click();
  await expect(page.locator('.thumb-checkbox.is-selected')).toHaveCount(2);
  await cards.nth(2).locator('.thumb-open').click({ modifiers: ['Shift'] });
  await expect(page.locator('.thumb-checkbox.is-selected')).toHaveCount(4);
  await expect(cards.nth(0).locator('.thumb-checkbox')).toHaveClass(/is-selected/);
  await expect(cards.nth(2).locator('.thumb-checkbox')).toHaveClass(/is-selected/);
  await expect(cards.nth(3).locator('.thumb-checkbox')).toHaveClass(/is-selected/);
  await expect(cards.nth(4).locator('.thumb-checkbox')).toHaveClass(/is-selected/);
});
