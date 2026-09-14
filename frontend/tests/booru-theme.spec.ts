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

type MockOptions = {
  accentColor?: string;
  fontStyle?: 'editorial' | 'modern' | 'comic';
  fontStyleConfigured?: boolean;
  authenticated?: boolean;
};

async function mockThemeApp(
  page: Page,
  theme: 'default' | 'booru-light' | 'booru-dark' = 'booru-light',
  options: MockOptions = {}
) {
  const {
    accentColor = '',
    fontStyle = 'comic',
    fontStyleConfigured = false,
    authenticated = true
  } = options;

  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({
      ui_theme: theme,
      accent_color: accentColor,
      font_style: fontStyle,
      font_style_configured: fontStyleConfigured,
      grid_size: 200,
      grid_type: 'square',
      pagination_mode: 'infinite',
      capabilities: ['preview_images']
    })
  }));
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(authenticated ? session : { user: null, csrf_token: '' })
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
  if (authenticated) await expect(page.getByRole('heading', { name: 'Library' })).toBeVisible();
  else await expect(page.getByRole('button', { name: 'Sign in' })).toBeVisible();
}

test('booru-light uses the shared booru shell with native fonts and yellow brand accent by default', async ({ page }) => {
  await mockThemeApp(page, 'booru-light');

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-theme-booru-style/);
  await expect(root).toHaveClass(/gooru-theme-booru-light/);
  await expect(root).not.toHaveClass(/gooru-theme-booru-dark/);
  await expect(root).not.toHaveClass(/gooru-type-/);
  await expect(root).toHaveAttribute('style', /--brand-accent:\s*#ffd060/);
  await expect(root).not.toHaveAttribute('style', /--accent:/);

  await expect(page.locator('.topbar')).toHaveCount(0);
  await expect(page.locator('.booru-brand')).toContainText('Gooru');
  await expect(page.locator('.booru-brand')).toHaveCSS('font-family', /Tahoma/);
  await expect(page.locator('.booru-brand-mark')).toHaveCount(1);
  await expect(page.locator('.booru-brand-mark')).toHaveCSS('background-color', 'rgb(255, 208, 96)');
  await expect(page.locator('.booru-main-nav')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.booru-subnav')).toHaveCSS('background-color', 'rgb(244, 246, 255)');
  await expect(page.locator('.booru-app-shell')).toHaveCSS('grid-template-columns', /288px/);
  await expect(page.locator('.booru-sidebar .searchbar')).toBeVisible();
  await expect(page.locator('.booru-sidebar .searchbar')).toHaveCSS('border-radius', '0px');

  const searchInput = page.locator('.booru-sidebar .searchbar-input');
  await expect(searchInput).toHaveCSS('font-family', /Verdana/);
  await expect(searchInput).toHaveAttribute('placeholder', '');
  await expect(page.locator('.booru-sidebar .searchbar-pill')).toHaveCount(0);
  await expect(page.locator('.booru-sidebar .searchbar-shortcut')).toHaveCount(0);
  await searchInput.fill('artist:demo rating:safe');
  await searchInput.press('Enter');
  await expect(searchInput).toHaveValue('artist:demo rating:safe');
  await expect(page.locator('.booru-sidebar .searchbar-pill')).toHaveCount(0);

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
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.locator('.lightbox')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.lightbox-stage')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.lightbox-aside')).toHaveCSS('background-color', 'rgb(255, 255, 255)');
  await expect(page.locator('.lightbox-name')).toHaveCSS('font-size', '14px');
  await expect(page.locator('.lightbox-rail .g-btn').first()).toHaveCSS('border-radius', '0px');
});

test('booru login uses the same yellow spiral brand accent by default', async ({ page }) => {
  await mockThemeApp(page, 'booru-light', { authenticated: false });

  const root = page.locator('.gooru-root');
  await expect(root).toHaveAttribute('style', /--brand-accent:\s*#ffd060/);
  await expect(page.locator('.login-v2-spirals path')).toHaveCSS('stroke', 'rgb(255, 208, 96)');
  await expect(page.locator('.login-v2-mark .gooru-logo-accent-fill')).toHaveCSS('fill', 'rgb(255, 208, 96)');
  await expect(page.locator('link[rel="icon"]').last()).toHaveAttribute('href', /\/favicon\.svg$/);
});

test('booru custom accent recolors only spiral branding and favicon', async ({ page }) => {
  await mockThemeApp(page, 'booru-light', { accentColor: '#0c2238' });

  const root = page.locator('.gooru-root');
  await expect(root).toHaveAttribute('style', /--brand-accent:\s*#0c2238/);
  await expect(root).not.toHaveAttribute('style', /--accent:\s*#0c2238/);
  await expect(page.locator('.booru-brand-mark')).toHaveCSS('background-color', 'rgb(12, 34, 56)');
  await expect(page.getByRole('navigation', { name: 'Primary navigation' }).getByRole('button', { name: 'Tags' })).toHaveCSS('color', 'rgb(0, 117, 248)');
  const booruAccent = await root.evaluate((node) => getComputedStyle(node).getPropertyValue('--accent').trim());
  expect(booruAccent).toBe('#0075f8');
  await expect(page.locator('link[rel="icon"]').last()).toHaveAttribute('href', /^data:image\/svg\+xml,/);
});

test('default theme keeps applying configured accent to the full UI token', async ({ page }) => {
  await mockThemeApp(page, 'default', { accentColor: '#0c2238', authenticated: false, fontStyleConfigured: true });

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-theme-default/);
  await expect(root).toHaveAttribute('style', /--brand-accent:\s*#0c2238/);
  await expect(root).toHaveAttribute('style', /--accent:\s*#0c2238/);
  const defaultAccent = await root.evaluate((node) => getComputedStyle(node).getPropertyValue('--accent').trim());
  expect(defaultAccent).toBe('#0c2238');
  await expect(page.locator('.login-v2-spirals path')).toHaveCSS('stroke', 'rgb(12, 34, 56)');
  await expect(page.locator('link[rel="icon"]').last()).toHaveAttribute('href', /^data:image\/svg\+xml,/);
});

test('booru explicit font style overrides native typography without changing palette', async ({ page }) => {
  await mockThemeApp(page, 'booru-light', { fontStyle: 'modern', fontStyleConfigured: true });

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-type-modern/);
  await expect(page.locator('.booru-brand')).toHaveCSS('font-family', /IBM Plex Sans/);
  await expect(page.locator('.booru-sidebar .searchbar-input')).toHaveCSS('font-family', /IBM Plex Sans/);
  await expect(page.locator('.booru-main-nav').getByRole('button', { name: 'Tags' })).toHaveCSS('color', 'rgb(0, 117, 248)');
});

test('booru-dark follows the committed reference dark palette on shell and viewer surfaces', async ({ page }) => {
  await mockThemeApp(page, 'booru-dark');

  const root = page.locator('.gooru-root');
  await expect(root).toHaveClass(/gooru-theme-booru-style/);
  await expect(root).toHaveClass(/gooru-theme-booru-dark/);
  await expect(root).not.toHaveClass(/gooru-type-/);
  await expect(root).not.toHaveAttribute('style', /--accent:/);
  await expect(root).toHaveCSS('background-color', 'rgb(30, 30, 44)');
  await expect(page.locator('.booru-main-nav')).toHaveCSS('background-color', 'rgb(30, 30, 44)');
  await expect(page.locator('.booru-subnav')).toHaveCSS('background-color', 'rgb(44, 45, 63)');
  await expect(page.locator('.booru-main-nav').getByRole('button', { name: 'Tags' })).toHaveCSS('color', 'rgb(0, 155, 230)');
  await expect(page.locator('.booru-nav-tab').first()).toHaveCSS('color', 'rgb(75, 180, 255)');
  await expect(page.locator('.booru-sidebar .searchbar')).toHaveCSS('background-color', 'rgb(63, 64, 88)');
  await expect(page.locator('.booru-sidebar .searchbar')).toHaveCSS('border-color', 'rgb(119, 120, 146)');
  await expect(page.locator('.booru-brand-mark')).toHaveCSS('background-color', 'rgb(255, 208, 96)');

  const card = page.locator('.thumb').filter({ has: page.getByAltText('sample.png') });
  await card.getByRole('button', { name: 'Preview sample.png' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  await expect(page.locator('.lightbox')).toHaveCSS('background-color', 'rgb(30, 30, 44)');
  await expect(page.locator('.lightbox-stage')).toHaveCSS('background-color', 'rgb(30, 30, 44)');
  await expect(page.locator('.lightbox-aside')).toHaveCSS('background-color', 'rgb(30, 30, 44)');
  await expect(page.locator('.lightbox-tag-list .g-tag').first()).toHaveCSS('color', 'rgb(0, 155, 230)');
});
