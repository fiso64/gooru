import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

function fileItem(id: 'booru-image' | 'other-image', name: string, tag: string) {
  return {
    id,
    content_id: `hash-${id}`,
    name,
    safe_display_path: `uploads/${name}`,
    size: 4096,
    added_at: '2026-09-09T08:00:00Z',
    modified_time: '2026-09-09T08:00:00Z',
    media_type: 'image/png',
    media_kind: 'image',
    metadata: { width: 640, height: 480 },
    tags: [tag],
    media_urls: {
      thumbnail: `/api/v1/files/${id}/thumbnail`,
      preview: `/api/v1/files/${id}/preview`,
      content: `/api/v1/files/${id}/content`,
      download: `/api/v1/files/${id}/download`
    },
    can_delete: false
  };
}

const demoFile = fileItem('booru-image', 'sample.png', 'artist:demo');
const otherFile = fileItem('other-image', 'other.png', 'artist:other');

async function mockDarkBooru(page: Page) {
  const fileRequests: string[] = [];

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
      library_count: 2,
      facets: { kind: [{ value: 'image', count: 2 }] }
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
  await page.route('**/api/v1/files?**', async (route) => {
    const requestURL = route.request().url();
    fileRequests.push(requestURL);
    const filtered = decodeURIComponent(requestURL).includes('artist:demo');
    const files = filtered ? [demoFile] : [demoFile, otherFile];
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ files, total_count: files.length, library_count: 2, facets: { kind: [{ value: 'image', count: files.length }] } })
    });
  });
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="640" height="480"><rect width="640" height="480"/></svg>';
  await page.route('**/api/v1/files/*/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  return { fileRequests };
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

test('text-mode suggestion insertion leaves a separator and empty Enter restores root results', async ({ page }) => {
  const { fileRequests } = await mockDarkBooru(page);
  const input = page.locator('.booru-sidebar .searchbar-input');
  const cards = page.locator('.thumb');

  await expect(cards).toHaveCount(2);
  await input.fill('artist:d');
  await expect(page.locator('.booru-sidebar .search-suggestions')).toBeVisible();
  await input.press('Enter');

  await expect(input).toHaveValue('artist:demo ');
  await expect(cards).toHaveCount(1);
  await expect.poll(() => decodeURIComponent(fileRequests.at(-1) ?? '')).toContain('artist:demo');

  await input.press('Control+A');
  await input.press('Delete');
  await input.press('Enter');

  await expect(input).toHaveValue('');
  await expect(cards).toHaveCount(2);
  await expect(page.getByAltText('sample.png')).toBeVisible();
  await expect(page.getByAltText('other.png')).toBeVisible();
});

test('empty library does not advertise the unavailable import command', async ({ page }) => {
  await mockDarkBooru(page);
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
  await page.reload();

  const emptyState = page.locator('.empty-state');
  await expect(emptyState).toContainText('Your library is empty. Drag files in.');
  await expect(emptyState).not.toContainText('gooru import');
});
