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
  tags: ['artist:demo', 'rating:safe'],
  media_urls: {
    thumbnail: '/api/v1/files/booru-image/thumbnail',
    preview: '/api/v1/files/booru-image/preview',
    content: '/api/v1/files/booru-image/content',
    download: '/api/v1/files/booru-image/download'
  },
  can_delete: false
};

async function mockBooruApp(page: Page) {
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      ui_theme: 'booru-style',
      accent_color: '#0c2238',
      font_style: 'comic',
      grid_size: 200,
      grid_type: 'square',
      capabilities: ['preview_images']
    })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  for (const path of ['jobs', 'saved-searches']) {
    await page.route(`**/api/v1/${path}`, async (route) => route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ items: [] })
    }));
  }
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ items: [] })
  }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ tags: [], library_count: 1, facets: { kind: [{ value: 'image', count: 1 }] } })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({
    contentType: 'application/json', body: JSON.stringify({ items: [], meta_tags: [] })
  }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [file], total_count: 1, library_count: 1, facets: { kind: [{ value: 'image', count: 1 }] } })
  }));
  const svg = '<svg xmlns="http://www.w3.org/2000/svg" width="640" height="480"><rect width="640" height="480" fill="#ddd"/></svg>';
  await page.route('**/api/v1/files/booru-image/thumbnail', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));
  await page.route('**/api/v1/files/booru-image/preview', async (route) => route.fulfill({ contentType: 'image/svg+xml', body: svg }));

  await page.goto('/');
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
}

test('runtime booru theme uses a real booru top navigation and shared route behavior', async ({ page }) => {
  await mockBooruApp(page);

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-theme-booru-style/);
  await expect(root).toHaveClass(/gooru-type-modern/);
  await expect(root).not.toHaveAttribute('style', /--accent:/);

  await expect(page.locator('.topbar')).toHaveCount(0);
  await expect(page.locator('.booru-brand')).toContainText('Gooru');
  await expect(page.locator('.booru-brand-mark')).toHaveCount(1);
  await expect(page.locator('.booru-brand-mark')).toHaveAttribute('src', '/favicon.svg');
  await expect(page.locator('.booru-main-nav')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.booru-subnav')).toHaveCSS('background-color', 'rgb(244, 246, 255)');
  await expect(page.locator('.booru-sidebar .searchbar')).toBeVisible();
  await expect(page.locator('.booru-sidebar .searchbar')).toHaveCSS('border-radius', '0px');
  await expect(page.locator('.library-head h1')).toHaveCSS('font-size', '18px');
  await expect(page.locator('.sidebar-item').first()).toHaveCSS('border-radius', '0px');

  const primaryNav = page.getByRole('navigation', { name: 'Primary navigation' });
  const libraryTab = primaryNav.getByRole('button', { name: 'Library' });
  await expect(libraryTab).toHaveAttribute('aria-current', 'page');
  await primaryNav.getByRole('button', { name: 'Tags' }).click();
  await expect(page.getByRole('heading', { name: '0 tags across 1 files' })).toBeVisible();
  await expect(primaryNav.getByRole('button', { name: 'Tags' })).toHaveAttribute('aria-current', 'page');
  await libraryTab.click();
  await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();

  const card = page.locator('.thumb').filter({ has: page.getByAltText('sample.png') });
  await expect(card).toBeVisible();
  await expect(card.locator('img')).toHaveCSS('object-fit', 'contain');

  const gridGeometry = await page.getByTestId('virtual-media-grid').evaluate((node) => {
    const style = getComputedStyle(node);
    const columns = style.gridTemplateColumns.split(/\s+/).map((value) => Number.parseFloat(value)).filter(Number.isFinite);
    const gap = Number.parseFloat(style.columnGap) || 0;
    const paddingLeft = Number.parseFloat(style.paddingLeft) || 0;
    const paddingRight = Number.parseFloat(style.paddingRight) || 0;
    return {
      columns: columns.length,
      clientWidth: node.clientWidth,
      usedWidth: columns.reduce((total, width) => total + width, 0) + gap * Math.max(0, columns.length - 1) + paddingLeft + paddingRight
    };
  });
  expect(gridGeometry.columns).toBeGreaterThan(1);
  expect(Math.abs(gridGeometry.clientWidth - gridGeometry.usedWidth)).toBeLessThanOrEqual(1);

  await card.getByRole('button', { name: 'Preview sample.png' }).click();

  const viewer = page.getByRole('dialog');
  await expect(viewer).toBeVisible();
  await expect(page.locator('.lightbox')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.lightbox-stage')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.lightbox-aside')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.lightbox-name')).toHaveCSS('font-size', '14px');
  await expect(page.locator('.lightbox-rail .g-btn').first()).toHaveCSS('border-radius', '0px');
  await expect(root).toHaveClass(/gooru-theme-booru-style/);
});
