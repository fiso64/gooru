import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

const file = {
  id: 'booru-image',
  content_id: 'hash-booru-image',
  name: 'sample.png',
  safe_display_path: 'uploads/sample.png',
  size: 4096,
  added_at: '2026-09-09T08:00:00Z',
  modified_time: '2026-09-09T08:00:00Z',
  media_type: 'image/png',
  media_kind: 'image',
  metadata: { width: 640, height: 480 },
  tags: ['artist:demo'],
  media_urls: {
    thumbnail: '/api/v1/files/booru-image/thumbnail',
    preview: '/api/v1/files/booru-image/preview',
    content: '/api/v1/files/booru-image/content',
    download: '/api/v1/files/booru-image/download'
  },
  can_delete: false
};

async function mockDarkBooru(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      ui_theme: 'booru-dark',
      grid_size: 180,
      grid_type: 'fit',
      pagination_mode: 'paged',
      items_per_page: 50,
      capabilities: ['preview_images']
    })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify(session)
  }));
  for (const path of ['jobs', 'saved-searches']) {
    await page.route(`**/api/v1/${path}`, async (route) => route.fulfill({
      contentType: 'application/json', body: JSON.stringify({ items: [] })
    }));
  }
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      tags: [
        { name: 'artist:demo', count: 42 },
        { name: 'artist:other', count: 17 }
      ],
      library_count: 1,
      facets: { kind: [{ value: 'image', count: 1 }] }
    })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      items: [
        { name: 'artist:demo', count: 42 },
        { name: 'artist:other', count: 17 }
      ],
      meta_tags: []
    })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [{ value: 'image', count: 1 }] } })
  }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="640" height="480"><rect width="640" height="480"/></svg>';
  await page.route('**/api/v1/files/booru-image/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('booru search feedback presentation is compact and selected row is visible in dark theme', async ({ page }) => {
  await mockDarkBooru(page);

  await expect(page.locator('.booru-app-shell')).toHaveCSS('grid-template-columns', /288px/);

  const input = page.locator('.booru-sidebar .searchbar-input');
  await input.fill('arti');

  const suggestions = page.locator('.booru-sidebar .search-suggestions');
  await expect(suggestions).toBeVisible();
  await expect(suggestions.locator('.group-head')).toBeHidden();
  await expect(suggestions.locator('.search-suggestions-footer')).toBeHidden();

  const active = suggestions.getByRole('option').first();
  await expect(active).toHaveAttribute('aria-selected', 'true');
  await expect(active).toHaveCSS('background-color', 'rgb(63, 64, 88)');
  await expect(active.locator('.hint')).toBeHidden();
  await expect(active.locator('.count')).toBeVisible();
});
