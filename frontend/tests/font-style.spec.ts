import { expect, test, type Page } from '@playwright/test';

const session = {
  user: { id: 'usr_test', username: 'mac', role: 'admin' },
  capabilities: { upload: true, tag: true, delete: true, admin: true },
  csrf_token: 'csrf-one'
};

async function mockLoggedOutSession(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ error: { code: 'unauthorized', message: 'login required' } })
    });
  });
}

async function mockAuthenticatedLibrary(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify(session)
  }));
  await page.route('**/api/v1/saved-searches', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/upload-targets', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/tags?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ tags: [], library_count: 0, facets: { kind: [] } })
  }));
  await page.route('**/api/v1/search/suggestions?**', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({ items: [] }) }));
  await page.route('**/api/v1/files?**', async (route) => route.fulfill({
    contentType: 'application/json',
    body: JSON.stringify({ files: [], total_count: 0, library_count: 0, facets: { kind: [] } })
  }));
}

async function measuredTextWidth(page: Page, family: string, weight: string) {
  return await page.evaluate(({ fontFamily, fontWeight }) => {
    const span = document.createElement('span');
    span.textContent = 'Library';
    Object.assign(span.style, {
      position: 'absolute',
      visibility: 'hidden',
      whiteSpace: 'nowrap',
      fontFamily,
      fontSize: '32px',
      fontWeight,
      letterSpacing: '-0.32px'
    });
    document.body.appendChild(span);
    const width = span.getBoundingClientRect().width;
    span.remove();
    return width;
  }, { fontFamily: family, fontWeight: weight });
}

test('comic font preset uses the bundled face for headings and wordmark letters while preserving the regular spiral Os', async ({ page }) => {
  await mockAuthenticatedLibrary(page);
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ font_style: 'comic', load_full_media_by_default: false })
    });
  });

  await page.goto('/');

  const root = page.locator('.gooru-root');
  const heading = page.locator('.library-head h1');
  await expect(root).toHaveClass(/gooru-type-comic/);
  await expect(heading).toHaveText('Library');
  await page.evaluate(() => document.fonts.ready);

  const typography = await heading.evaluate(async (element) => {
    const style = getComputedStyle(element);
    const faces = await document.fonts.load(`${style.fontWeight} ${style.fontSize} "Comic Neue"`, element.textContent ?? 'Library');
    return {
      loadedFaces: faces.length,
      family: style.fontFamily,
      weight: style.fontWeight,
      width: element.getBoundingClientRect().width
    };
  });
  const comicWidth = await measuredTextWidth(page, '"Comic Neue"', '700');

  expect(typography.loadedFaces).toBeGreaterThan(0);
  expect(typography.family).toMatch(/^"?Comic Neue"?/);
  expect(typography.weight).toBe('700');
  expect(Math.abs(typography.width - comicWidth)).toBeLessThan(0.75);

  const comicLogo = page.locator('.gooru-logo-comic');
  const comicLetters = comicLogo.locator('.gooru-logo-comic-letter');
  await expect(comicLogo).toBeVisible();
  await expect(comicLetters).toHaveCount(3);
  await expect(page.locator('.gooru-logo-default')).toBeHidden();
  await expect(page.locator('.gooru-logo-accent')).toBeVisible();
  await expect(page.locator('.gooru-logo-accent image[href="/favicon.svg"]')).toHaveCount(2);

  const logoTypography = await comicLetters.first().evaluate(async (element) => {
    const style = getComputedStyle(element);
    const faces = await document.fonts.load(`${style.fontWeight} ${style.fontSize} "Comic Neue"`, element.textContent ?? 'g');
    return { family: style.fontFamily, weight: style.fontWeight, loadedFaces: faces.length };
  });
  expect(logoTypography.family).toMatch(/^"?Comic Neue"?/);
  expect(logoTypography.weight).toBe('700');
  expect(logoTypography.loadedFaces).toBeGreaterThan(0);
});

test('modern preset uses a semibold sans display face without changing the UI face', async ({ page }) => {
  await mockAuthenticatedLibrary(page);
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ font_style: 'modern', load_full_media_by_default: false })
    });
  });

  await page.goto('/');

  const root = page.locator('.gooru-root');
  const heading = page.locator('.library-head h1');
  await expect(root).toHaveClass(/gooru-type-modern/);
  const variables = await root.evaluate((element) => {
    const style = getComputedStyle(element);
    return {
      display: style.getPropertyValue('--font-display'),
      displayWeight: style.getPropertyValue('--font-display-weight'),
      ui: style.getPropertyValue('--font-ui')
    };
  });
  expect(variables.display).toContain('IBM Plex Sans');
  expect(variables.displayWeight.trim()).toBe('600');
  expect(variables.ui).toContain('IBM Plex Sans');
  await expect(heading).toHaveCSS('font-weight', '600');
  await expect(page.locator('.gooru-logo-default')).toBeVisible();
  await expect(page.locator('.gooru-logo-comic')).toBeHidden();
  await expect(page.locator('.gooru-logo-accent')).toBeVisible();
});

test('editorial preset keeps the regular serif heading weight', async ({ page }) => {
  await mockAuthenticatedLibrary(page);
  await page.route('**/api/v1/ui-config', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ font_style: 'editorial', load_full_media_by_default: false })
    });
  });

  await page.goto('/');

  const heading = page.locator('.library-head h1');
  await expect(heading).toHaveCSS('font-weight', '400');
  await expect(page.locator('.gooru-logo-default')).toBeVisible();
  await expect(page.locator('.gooru-logo-comic')).toBeHidden();
  await expect(page.locator('.gooru-logo-accent')).toBeVisible();
});

test('missing font_style defaults the web UI to comic', async ({ page }) => {
  await mockAuthenticatedLibrary(page);
  await page.route('**/api/v1/ui-config', async (route) => route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) }));
  await page.goto('/');
  await expect(page.locator('.gooru-root')).toHaveClass(/gooru-type-comic/);
});
